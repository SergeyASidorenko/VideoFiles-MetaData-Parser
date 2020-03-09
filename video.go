// Copyright 2020 The Home. All rights not reserved.
// Пакет с реализацией извлечнеия общей метаинформации о видеофайле в формате .mp4
// Сведения о лицензии отсутствуют

// Получение метаинформации о видеопотоке/видеофайле, содержимое которого передается в теле запроса по HTTP протоколу
package main

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"log"
	"time"
)

// Константы типа потоков
const (
	Audio           string = "Audio Media"  // аудиопоток
	Video           string = "Visual Media" // видеопоток
	Hint            string = "Hint"         // поток-наводка (подсказка)
	HeaderBlockSize        = 0x8            // размер заголовка блока
)

var streamTypes = map[string]string{"soun": "Audio Media", "vide": "Visual Media"}
var codecs = map[string]string{"isom": "ISO 14496-1 Base Media", "iso2": "ISO 14496-12 Base Media", "mp41": "ISO 14496-1 vers. 1", "mp42": "ISO 14496-1 vers. 2"}

// VideoFile Структура для хранения метаинформации о видеофайле
type VideoFile struct {
	buffer       *bufio.Reader // буфер всего файла
	metaDataBuf  *bytes.Reader // буфер с метаданными
	offset       int64
	startOfBlock int64
	Size         int64     // размер файла
	Codec        string    // стандарт используемого сжатия видео и аудио потоков
	Movie        Container // видеоконтейнер
}

// Container Структура для хранения метаинформации о видеоконтейнере
type Container struct {
	durationFlag  byte
	Created       time.Time // Время создания
	Modified      time.Time // Время изменения
	TimeScale     uint32    // единица времени, используемая для квантования
	Duration      float64   // продолжительность медиа-данных в контейнере
	PlayBackSpeed uint16    // скорость воспроизведения
	Volume        string    // уровень звука (относительный)
	Tracks        []Track   // медиа-дорожки, содержащиеся в контейнере
}

// Track Структура для хранения метаинформации о медиа-дорожке
type Track struct {
	durationFlag byte
	Created      time.Time // Время создания
	Modified     time.Time // Время изменения
	Duration     float64   // продолжительность медиа-дорожки
	Height       uint32    // высота для дорожки видеопотока
	Width        uint32    // ширина для дорожки видеопотока
	Stream       Streamer  // поток данных, с которой связан данная дорожка
}

// Streamer интерфейс потока данных (их может быть аж до 9 типов, в нашем случае - только два)
type Streamer interface {
	Read(buf *bytes.Reader) error
	GetType() string
}

// Stream общее описание потоков, заголовочная часть 'minf',
// например, здесь описываются все возможные типы потоков
//	============================================================
// 	ПОТОКИ
// 	Visual Media = 'vide';
// 	Audio Media = 'soun';
// 	Hint = "hint';
// 	Object Descriptor = 'odsm';
// 	Clock Reference = 'crsm';
// 	Scene Description = 'sdsm';
// 	MPEG-7 Stream = 'm7sm';
// 	Object Content Info = 'ocsm';
// 	IPMP = 'ipsm' : MPEG-J = 'mjsm';
type Stream struct {
	durationFlag byte
	TimeScale    uint32  // Частота дискретизации
	Duration     float64 // Продолжительность
	Type         string  // Тип потока
}

// Информация о потоках представлена в секции 'minf'
// Также есть два дополнительных блока информации для следующих потоков
// Hint  в секции 'hint'
// mpeg-4 media в секции 'nmhd'

// AudioStream данные аудиопотока
type AudioStream struct {
	*Stream
	AudioBalance string // Баланс
	Format       string // Формат
	Channels     string // Количество каналов
	SampleRate   uint32 // Частота дискретизации
}

// VideoStream данные видеопотока
type VideoStream struct {
	*Stream
	Format     string // Формат
	ResY       uint16 // Разрешение по вертикали
	ResX       uint16 // Разрешение по вертикали
	ColorDepth uint16 // Глубина цвета
}

// Prepare получение значения последнего элемента
func (f *VideoFile) Prepare() (temp []byte, err error) {
	// Если блок прочитан полностью - смещать на начало следующего блока не надо
	// для этого используется флаг isBlockRead
	var isBlockRead bool
	// наименование блока
	var box string
	// длина блока в байтах
	var offset int64
	var boxInfo = make([]byte, HeaderBlockSize)
	sectors := map[string]string{"ftyp": "", "moov": ""}
	for {
		isBlockRead = false
		_, err = io.ReadFull(f.buffer, boxInfo)
		if err == io.EOF {
			break
		}
		Fatal(err)
		offset = int64(binary.BigEndian.Uint32(boxInfo[:4]))
		f.Size += offset
		box = string(boxInfo[4:HeaderBlockSize])
		if _, ok := sectors[box]; ok {
			var b = make([]byte, offset-HeaderBlockSize)
			_, err = io.ReadFull(f.buffer, b)
			Fatal(err)
			temp = append(temp, boxInfo...)
			temp = append(temp, b...)
			isBlockRead = true
		}
		if !isBlockRead {
			_, err = f.buffer.Discard(int(offset - HeaderBlockSize))
		}
		Fatal(err)
	}
	return temp, nil
}

// Parse Метод разбора видеофайла на метаданные
func (f *VideoFile) Parse() (err error) {
	defer Restore(&err, "ошибка парсинга видеофайла")
	// наименование блока
	var box string
	// длина блока в байтах
	cur := make([]byte, 8)
	f.startOfBlock, err = f.metaDataBuf.Seek(0, io.SeekCurrent)
	Fatal(err)
	_, err = f.metaDataBuf.Read(cur)
	if err == io.EOF {
		return nil
	}
	Fatal(err)
	box = string(cur[4:8])
	f.offset = int64(binary.BigEndian.Uint32(cur[:4]))
	// Данный блок позволяет войти внутрь интересующего блока описания данных в
	// видеофайле, структура видеофайла - это дерево блоков,
	// каждый блок описывает определенную часть файла, например, блок медиаданных,
	// блок описания файла, бок описания контейнера и так далее
	// В зависимости от блока мы либо переходим в дочернему узлу (сразу повторно вызываем метод Parse)
	// либо переходим с смежному узлу (перемещаем указатель буфера на длину текущего блока)
	switch box {
	case "ftyp":
		err = f.ReadFileInfo()
		Fatal(err)
	case "mvhd":
		err = f.ReadContainer()
		Fatal(err)
	case "tkhd":
		err = f.ReadTrack()
		Fatal(err)
	case "mdhd":
		stream := f.GetCurrentTrack().Stream
		err = stream.Read(f.metaDataBuf)
		Fatal(err)
	case "smhd":
		aStream := new(AudioStream)
		aStream.Stream = f.GetCurrentTrack().Stream.(*Stream)
		f.GetCurrentTrack().Stream = aStream
		err = aStream.Read(f.metaDataBuf)
		Fatal(err)
	case "vmhd":
		vStream := new(VideoStream)
		vStream.Stream = f.GetCurrentTrack().Stream.(*Stream)
		f.GetCurrentTrack().Stream = vStream
	case "stsd":
		f.ReadStreamExtraInfo()
	case "trak":
		f.Parse()
	case "mdia":
		fallthrough
	case "minf":
		fallthrough
	case "stbl":
		fallthrough
	case "moov":
		return f.Parse()
	}
	err = f.SeekBlockEnd()
	Fatal(err)
	return f.Parse()
}

// Open Метод проверки доступности и корректности файла, создание буфера и.т.д
func (f *VideoFile) Open(r io.Reader) (err error) {
	defer Restore(&err, "ошибка парсинга видеофайла")
	f.buffer = bufio.NewReaderSize(r, 0xFFFF)
	temp, err := f.Prepare()
	Fatal(err)
	f.buffer.Reset(nil)
	f.metaDataBuf = bytes.NewReader(temp)
	return nil
}

// ToJSON сериализация метаданных в формат JSON
func (f VideoFile) ToJSON() ([]byte, error) {
	b, err := json.Marshal(f)
	if err != nil {
		return nil, err
	}
	return b, nil
}

// getDateFromBytes Получения даты по набору байтов
func (f VideoFile) getDateFromMP4(data []byte) (time.Time, error) {
	macStartTime := time.Date(1904, 1, 1, 0, 0, 0, 0, time.UTC)
	if len(data) == 4 {
		return macStartTime.Add(time.Duration(binary.BigEndian.Uint32(data)) * time.Second), nil
	} else if len(data) == 8 {
		return macStartTime.Add(time.Duration(binary.BigEndian.Uint64(data)) * time.Second), nil
	}
	return time.Time{}, errors.New("неизвестный формат даты")
}

// SeekBlockEnd Перескок в конец текущего раздела видеофайла и очистка буфера
func (f *VideoFile) SeekBlockEnd() (err error) {
	curPos, err := f.metaDataBuf.Seek(0, io.SeekCurrent)
	if err != nil {
		return err
	}
	_, err = f.metaDataBuf.Seek(f.offset-(curPos-f.startOfBlock), io.SeekCurrent)
	if err != nil {
		return err
	}
	return nil
}

// ReadFileInfo Чтение общей информации о видеофайле
func (f *VideoFile) ReadFileInfo() (err error) {
	var temp = make([]byte, 4)
	_, err = f.metaDataBuf.Read(temp)
	if err != nil {
		return err
	}
	brand := string(temp)
	if !f.IsSupported(brand) {
		return errors.New("формат видеофайла неизвестен или не поддерживается")
	}
	f.Codec = codecs[brand]
	return nil
}

// SendError Отправка ошибки в формате JSON
func (f *VideoFile) SendError(e error) (b []byte, err error) {
	var apiErr APIError
	if !errors.As(e, &apiErr) {
		apiErr = NewAPIError("ошибка на стороне сервера", e)
	}
	b, err = json.Marshal(apiErr)
	if err != nil {
		return nil, err
	}
	log.Println(apiErr.Error())
	return b, nil
}

// IsSupported Проверка формата видеофайла (поддерживается или нет)
func (f *VideoFile) IsSupported(brand string) bool {
	_, ok := codecs[brand]
	return ok
}

// ReadContainer Чтение общей информации о видеоконтейнере
func (f *VideoFile) ReadContainer() (err error) {
	// подготавливаем буферы для чтения различных полей (разной длины)
	var temp2 = make([]byte, 2)
	var temp4 = make([]byte, 4)
	var temp8 = make([]byte, 8)
	var temp16 = make([]byte, 16)
	Fatal(err)
	f.Movie.durationFlag, err = f.metaDataBuf.ReadByte()
	Fatal(err)
	_, err = f.metaDataBuf.Seek(3, io.SeekCurrent) // пропускаем три байта
	Fatal(err)
	if f.Movie.durationFlag == 0x1 {
		_, err = f.metaDataBuf.Read(temp16)
		Fatal(err)
		f.Movie.Created, err = f.getDateFromMP4(temp16[:8])
		Fatal(err)
		f.Movie.Modified, err = f.getDateFromMP4(temp16[8:16])
		Fatal(err)
	} else {
		_, err = f.metaDataBuf.Read(temp8)
		Fatal(err)
		f.Movie.Created, err = f.getDateFromMP4(temp8[:4])
		Fatal(err)
		f.Movie.Modified, err = f.getDateFromMP4(temp8[4:8])
		Fatal(err)
	}
	_, err = f.metaDataBuf.Read(temp4)
	Fatal(err)
	f.Movie.TimeScale = binary.BigEndian.Uint32(temp4)
	if f.Movie.durationFlag == 0x1 {
		_, err = f.metaDataBuf.Read(temp8)
		Fatal(err)
		// Получение продолжительности в долях секунды
		duration := time.Duration(1000*binary.BigEndian.Uint64(temp8)/uint64(f.Movie.TimeScale)) * time.Millisecond
		f.Movie.Duration = duration.Seconds()

	} else {
		_, err = f.metaDataBuf.Read(temp4)
		Fatal(err)
		// Получение продолжительности в долях секунды
		duration := time.Duration(1000*binary.BigEndian.Uint32(temp4)/f.Movie.TimeScale) * time.Millisecond
		f.Movie.Duration = duration.Seconds()
	}
	_, err = f.metaDataBuf.Read(temp4)
	Fatal(err)
	f.Movie.PlayBackSpeed = binary.BigEndian.Uint16(temp4)
	_, err = f.metaDataBuf.Read(temp4)
	Fatal(err)
	volume := binary.BigEndian.Uint16(temp2)
	f.Movie.Volume = "normal"
	if volume == 0.0 {
		f.Movie.Volume = "mute"
	} else if volume == 3.0 {
		f.Movie.Volume = "maximum"
	}
	return nil
}

// ReadTrack Чтение общей информации о медиа-дорожке
func (f *VideoFile) ReadTrack() (err error) {
	var temp4 = make([]byte, 4)
	var temp8 = make([]byte, 8)
	// Создаем пустую дорожку
	track := Track{}
	track.Stream = new(Stream)
	track.durationFlag, err = f.metaDataBuf.ReadByte()
	Fatal(err)
	_, err = f.metaDataBuf.Seek(3, io.SeekCurrent) // пропускаем три байта (флаги описания дорожки)
	Fatal(err)
	if track.durationFlag == 0x1 {
		_, err = f.metaDataBuf.Read(temp8)
		Fatal(err)
		track.Created, err = f.getDateFromMP4(temp8)
		Fatal(err)
		_, err = f.metaDataBuf.Read(temp8)
		Fatal(err)
		track.Modified, err = f.getDateFromMP4(temp8)
		Fatal(err)
	} else {
		_, err = f.metaDataBuf.Read(temp4)
		Fatal(err)
		track.Created, err = f.getDateFromMP4(temp4)
		Fatal(err)
		_, err = f.metaDataBuf.Read(temp4)
		Fatal(err)
		track.Modified, err = f.getDateFromMP4(temp4)
		Fatal(err)
	}
	_, err = f.metaDataBuf.Seek(8, io.SeekCurrent) // пропускаем 8 байт (4 track_id, 4 - зарезервированы)
	Fatal(err)
	if track.durationFlag == 0x1 {
		_, err = f.metaDataBuf.Read(temp8)
		Fatal(err)
		// Получение продолжительности в долях секунды
		duration := time.Duration(1000*binary.BigEndian.Uint64(temp8)/uint64(f.Movie.TimeScale)) * time.Millisecond
		track.Duration = duration.Seconds()
	} else {
		_, err = f.metaDataBuf.Read(temp4)
		Fatal(err)
		// Получение продолжительности в долях секунды
		duration := time.Duration(1000*binary.BigEndian.Uint32(temp4)/f.Movie.TimeScale) * time.Millisecond
		track.Duration = duration.Seconds()
	}
	_, err = f.metaDataBuf.Seek(50, io.SeekCurrent)
	Fatal(err)
	_, err = f.metaDataBuf.Read(temp8)
	Fatal(err)
	track.Width = binary.BigEndian.Uint32(temp8[:4])
	track.Height = binary.BigEndian.Uint32(temp8[4:8])
	f.Movie.Tracks = append(f.Movie.Tracks, track)
	return nil
}

// GetCurrentTrack Получение текущей обрабатываемой медиа-дорожки
func (f *VideoFile) GetCurrentTrack() (t *Track) {
	return &f.Movie.Tracks[len(f.Movie.Tracks)-1]
}

// ReadStreamExtraInfo Чтение дополнительной информации о потоке
func (f *VideoFile) ReadStreamExtraInfo() (err error) {
	_, err = f.metaDataBuf.Seek(8, io.SeekCurrent)
	Fatal(err)
	StreamType := f.GetCurrentTrack().Stream.GetType()
	if StreamType == Audio {
		audioStream := f.GetCurrentTrack().Stream.(*AudioStream)
		temp := make([]byte, 4)
		f.metaDataBuf.Seek(4, io.SeekCurrent)
		_, err = f.metaDataBuf.Read(temp)
		Fatal(err)
		audioStream.Format = string(temp)

		_, err = f.metaDataBuf.Seek(16, io.SeekCurrent)
		Fatal(err)
		temp = make([]byte, 2)
		_, err = f.metaDataBuf.Read(temp)
		Fatal(err)
		channels := binary.BigEndian.Uint16(temp)
		if channels == 1 {
			audioStream.Channels = "Mono"
		} else if channels == 2 {
			audioStream.Channels = "Stereo"
		}
		_, err = f.metaDataBuf.Seek(6, io.SeekCurrent)
		Fatal(err)
		temp = make([]byte, 4)
		_, err = f.metaDataBuf.Read(temp)
		Fatal(err)
		audioStream.SampleRate = binary.BigEndian.Uint32(temp) >> 16
	} else if StreamType == Video {
		videoStream := f.GetCurrentTrack().Stream.(*VideoStream)
		err = videoStream.Read(f.metaDataBuf)
		Fatal(err)
	}
	return nil
}

// GetType Получение типа текущего потока
func (stream *Stream) GetType() string {
	return stream.Type
}

// ReadStream Чтение данные о потоке
func (stream *Stream) Read(buf *bytes.Reader) (err error) {
	var temp = make([]byte, 4)
	stream.durationFlag, err = buf.ReadByte()
	Fatal(err)
	_, err = buf.Seek(3, io.SeekCurrent) // пропускаем три байта (флаги описания дорожки)
	Fatal(err)
	if stream.durationFlag == 0x1 {
		_, err = buf.Seek(16, io.SeekCurrent)
		Fatal(err)
	} else {
		_, err = buf.Seek(8, io.SeekCurrent)
		Fatal(err)
	}
	_, err = buf.Read(temp)
	Fatal(err)
	stream.TimeScale = binary.BigEndian.Uint32(temp)
	if stream.durationFlag == 0x1 {
		temp = make([]byte, 8)
		_, err = buf.Read(temp)
		Fatal(err)
		// Получение продолжительности в долях секунды
		duration := time.Duration(1000*binary.BigEndian.Uint64(temp)/uint64(stream.TimeScale)) * time.Millisecond
		stream.Duration = duration.Seconds()

	} else {
		_, err = buf.Read(temp)
		Fatal(err)
		// Получение продолжительности в долях секунды
		duration := time.Duration(1000*binary.BigEndian.Uint32(temp)/stream.TimeScale) * time.Millisecond
		stream.Duration = duration.Seconds()
	}
	_, err = buf.Seek(20, io.SeekCurrent)
	Fatal(err)
	_, err = buf.Read(temp)
	Fatal(err)
	stream.Type = streamTypes[string(temp)]
	return nil
}

// ReadAudioStream  Чтение информации об аудиопотоке
func (stream *AudioStream) Read(buf *bytes.Reader) (err error) {
	temp := make([]byte, 2)
	_, err = buf.Read(temp)
	Fatal(err)
	balance := int16(binary.BigEndian.Uint16(temp))
	if balance < 0 {
		stream.AudioBalance = "left"
	} else if balance == 0 {
		stream.AudioBalance = "normal"
	} else {
		stream.AudioBalance = "right"
	}
	return nil
}

// ReadVideoStream Чтение информации о видеопотоке
func (stream *VideoStream) Read(buf *bytes.Reader) (err error) {
	temp := make([]byte, 4)
	_, err = buf.Seek(4, io.SeekCurrent)
	Fatal(err)
	_, err = buf.Read(temp)
	Fatal(err)
	stream.Format = string(temp)
	_, err = buf.Seek(28, io.SeekCurrent)
	Fatal(err)
	temp = make([]byte, 8)
	_, err = buf.Read(temp)
	Fatal(err)
	stream.ResX = binary.BigEndian.Uint16(temp[:4])
	stream.ResY = binary.BigEndian.Uint16(temp[4:8])
	_, err = buf.Seek(38, io.SeekCurrent)
	Fatal(err)
	temp = make([]byte, 2)
	_, err = buf.Read(temp)
	Fatal(err)
	stream.ColorDepth = binary.BigEndian.Uint16(temp)
	return nil
}
