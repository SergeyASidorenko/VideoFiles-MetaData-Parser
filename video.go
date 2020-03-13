// Copyright 2020 Sergey Sidorenko. All rights not reserved.
// Пакет с реализацией модудя извлечения метаинформации видеофайла в формате mp4
// Сведения о лицензии отсутствуют

// Получение метаинформации о видеопотоке/видеофайле, содержимое которого передается как объект Reader
package main

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"time"
)

// Константы типа потоков
const (
	Audio string = "Audio Media"  // аудиопоток
	Video string = "Visual Media" // видеопоток
	Hint  string = "Hint"         // поток-наводка (подсказка)
)

// HeaderBlockSize размер заголовка блока
const HeaderBlockSize = 0x8

// описание типов потоков
var streamTypes = map[string]string{"soun": "Audio Media", "vide": "Visual Media"}

// стандарты описания алгоритмов сжатия потоков
var codecs = map[string]string{"isom": "ISO 14496-1 Base Media", "iso2": "ISO 14496-12 Base Media", "mp41": "ISO 14496-1 vers. 1", "mp42": "ISO 14496-1 vers. 2"}

// VideoFile Структура для хранения метаинформации о видеофайле
// Файл MP4 представляет собой древовидную структуру,
// узлы которой описывают определенную часть информации о файле, одни - в более общей форме (узлы дерева), другие -
// непосредственно - так называемые листья дерева видеофайла
// Термин "Блок" здесь используется как логический узел этого дерева и может рассматриваться как
// участок содержимого файла со специальным описанием (размером блока и индентификатором (именем)) и его
// содержимым, содержимое блока - это данные, несущие определенную информацию о видеофайле.
// Описание блока здесь именуется как заголовок блока
// Этот заголовок в большинстве случаев имеет размер 8 байт и всегда располагается в начале блока.
// Некоторые блоки могут иметь размер более 8 байт
// Размер блока включает в себя размер заголовка
type VideoFile struct {
	buffer       *bufio.Reader // буфер всего файла
	metaDataBuf  *bytes.Reader // буфер с метаданными
	blockSize    int64         // размера текущего блока (байт)
	startOfBlock int64         // позиция начала блока относительно начала потока данных (байт)
	Size         int64         // размер файла (байт)
	Codec        string        // стандарт используемого сжатия видео и аудио потоков
	Movie        Container     // видеоконтейнер
}

// Container Структура для хранения метаинформации о видеоконтейнере
type Container struct {
	durationFlag  byte      // флаг, описывающий формат представления дат в файле (либо 0x0 - дата храниться как 4 байта, либо 0x1 - как 8 байт)
	Created       time.Time // время создания
	Modified      time.Time // время изменения
	TimeScale     uint32    // единица времени, используемая для квантования (обычно доли секунды)
	Duration      float64   // продолжительность медиа-данных в контейнере (сек)
	PlayBackSpeed uint16    // скорость воспроизведения (смысл значения мне до сих пор непонятен)
	Volume        string    // уровень звука (относительный)
	Tracks        []Track   // медиа-дорожки, содержащиеся в контейнере
}

// Track Структура для хранения метаинформации о медиа-дорожке
type Track struct {
	durationFlag byte         // флаг, описывающий формат представления дат в файле (либо 0x0 - дата храниться как 4 байта, либо 0x1 - как 8 байт)
	Created      time.Time    // время создания
	Modified     time.Time    // время изменения
	Duration     float64      // продолжительность медиа-дорожки (сек)
	Height       uint32       // высота для дорожки видеопотока (пиксель)
	Width        uint32       // ширина для дорожки видеопотока (пиксель)
	Stream       StreamReader // медиапоток данных, с которым связана данная дорожка (одна дорожка - один поток)
}

// StreamReader интерфейс медиапотока данных (их может быть аж до 10 типов, в нашем случае - только два)
type StreamReader interface {
	Read(buf *bytes.Reader) error // чтение данных исключительно, касающихся медиапотока
	GetType() string              // получениет типа потока
}

// Stream общее описание потока, блок с именем 'minf',
// ============================================================
// ПОТОКИ
// Visual Media = 'vide';
// Audio Media = 'soun';
// Hint = "hint';
// Object Descriptor = 'odsm';
// Clock Reference = 'crsm';
// Scene Description = 'sdsm';
// MPEG-7 Stream = 'm7sm';
// Object Content Info = 'ocsm';
// IPMP = 'ipsm';
// MPEG-J = 'mjsm';
type Stream struct {
	durationFlag byte    // флаг, описывающий формат представления дат в файле (либо 0x0 - дата храниться как 4 байта, либо 0x1 - как 8 байт)
	TimeScale    uint32  // частота сэмплирования (для видео = велчина кадров в секунду ; для аудио = количество сэмлов в секунду)
	Duration     float64 // продолжительность (сек)
	Type         string  // тип потока
}

// Информация о потоках представлена в секции 'minf'
// Также есть два дополнительных блока информации для следующих потоков
// Hint  в секции 'hint'
// mpeg-4 media в секции 'nmhd'

// AudioStream данные аудиопотока
type AudioStream struct {
	*Stream
	AudioBalance string // баланс
	Format       string // формат
	Channels     string // количество каналов (моно, стерео, ...)
	SampleRate   uint32 // частота дискретизации (Гц)
}

// VideoStream данные видеопотока
type VideoStream struct {
	*Stream
	Format     string // формат
	ResY       uint16 // разрешение по вертикали (точек на дюйм)
	ResX       uint16 // разрешение по горизонтали (точек на дюйм)
	ColorDepth uint16 // глубина цвета (бит)
}

// Prepare получение значения последнего элемента
func (f *VideoFile) Prepare() (temp []byte, err error) {
	// TODO
	// Блок требует доработки, уж очень недостоверная проверка соответствия формату MP4 злесь

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
		// дополнительная обработка блока медиаданных
		//  здесь формат заголовка может быть другим
		// в случае больших файлов под размер блока может отводиться не 4 а 8 байтов, а
		// иногда (если длина этого блока указана как 0x0)
		// данные этого блока продолжаются аж до конца файла
		if box == "mdat" {
			if offset == 0x1 {
				var extraBytesForBlockSize int64 = 0x8
				temp8 := make([]byte, 8)
				_, err = io.ReadFull(f.buffer, temp8)
				offset = int64(binary.BigEndian.Uint64(temp8)) - extraBytesForBlockSize
				Fatal(err)
			} else if offset == 0x0 {
				return
			}
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
	f.blockSize = int64(binary.BigEndian.Uint32(cur[:4]))
	// Данный блок позволяет войти внутрь интересующего блока описания данных в
	// видеофайле, структура видеофайла - это дерево блоков,
	// каждый блок описывает определенную часть файла, например, блок медиаданных,
	// блок описания файла, бок описания контейнера и так далее
	// В зависимости от блока мы либо переходим в дочернему узлу (сразу повторно вызываем метод Parse)
	// либо переходим с смежному узлу (перемещаем указатель буфера на длину текущего блока)
	switch box {
	case "ftyp":
		err = f.readFileInfo()
		Fatal(err)
	case "mvhd":
		err = f.readContainer()
		Fatal(err)
	case "tkhd":
		err = f.readTrack()
		Fatal(err)
	case "mdhd":
		stream := f.getCurrentTrack().Stream
		err = stream.Read(f.metaDataBuf)
		Fatal(err)
	case "smhd":
		aStream := new(AudioStream)
		aStream.Stream = f.getCurrentTrack().Stream.(*Stream)
		f.getCurrentTrack().Stream = aStream
		err = aStream.Read(f.metaDataBuf)
		Fatal(err)
	case "vmhd":
		vStream := new(VideoStream)
		vStream.Stream = f.getCurrentTrack().Stream.(*Stream)
		f.getCurrentTrack().Stream = vStream
	case "stsd":
		f.readStreamExtraInfo()

	// следующие инструкции позволяют вызвать сразу рекурсивно метод Parse
	// без перемещения указателя на конец этого блока,
	// таким образом мы как бы заходим внутрь узлов с нижеперечисленными именами
	case "trak":
		// в дереве файла может быть несколько узлов 'trak' (несколько медиадорожек), поэтому после рекурсивного вызова
		// мы ждем возврата и перескакиваем на конец текущего трека, в надежде найти следующий блок 'trak'
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
	// перемещаемся на позицию конца текущего блока
	err = f.seekBlockEnd()
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

// seekBlockEnd Перескок в конец текущего раздела видеофайла и очистка буфера
func (f *VideoFile) seekBlockEnd() (err error) {
	curPos, err := f.metaDataBuf.Seek(0, io.SeekCurrent)
	if err != nil {
		return err
	}
	_, err = f.metaDataBuf.Seek(f.blockSize-(curPos-f.startOfBlock), io.SeekCurrent)
	if err != nil {
		return err
	}
	return nil
}

// readFileInfo Чтение общей информации о видеофайле
func (f *VideoFile) readFileInfo() (err error) {
	var temp = make([]byte, 4)
	_, err = f.metaDataBuf.Read(temp)
	if err != nil {
		return err
	}
	brand := string(temp)
	if !f.isSupported(brand) {
		return errors.New("формат видеофайла неизвестен или не поддерживается")
	}
	f.Codec = codecs[brand]
	return nil
}

// GetError Выдача описания ошибки сервиса вышестоящим потребителям
func (f *VideoFile) GetError(e error) *APIError {
	var apiErr APIError
	if !errors.Is(e, &apiErr) {
		apiErr = NewAPIError("ошибка на стороне сервера", e)
	}
	return &apiErr
}

// isSupported Проверка формата видеофайла (поддерживается или нет)
func (f *VideoFile) isSupported(brand string) bool {
	_, ok := codecs[brand]
	return ok
}

// readContainer Чтение общей информации о видеоконтейнере
func (f *VideoFile) readContainer() (err error) {
	defer Restore(&err, "ошибка чтения метаданных контейнера")
	// подготавливаем буферы для чтения различных полей (разной длины)
	var temp2 = make([]byte, 2)
	var temp4 = make([]byte, 4)
	var temp8 = make([]byte, 8)
	var temp16 = make([]byte, 16)
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

// readTrack Чтение общей информации о медиа-дорожке
func (f *VideoFile) readTrack() (err error) {
	defer Restore(&err, "ошибка чтения метаданных медиадорожки")
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

// getCurrentTrack Получение текущей обрабатываемой медиа-дорожки
func (f *VideoFile) getCurrentTrack() *Track {
	return &f.Movie.Tracks[len(f.Movie.Tracks)-1]
}

// readStreamExtraInfo Чтение дополнительной информации о потоке
func (f *VideoFile) readStreamExtraInfo() (err error) {
	defer Restore(&err, "ошибка чтения дополнительных метаданных медиапотока")
	_, err = f.metaDataBuf.Seek(8, io.SeekCurrent)
	Fatal(err)
	StreamType := f.getCurrentTrack().Stream.GetType()
	if StreamType == Audio {
		audioStream := f.getCurrentTrack().Stream.(*AudioStream)
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
		audioStream.Channels = "undefined"
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
		videoStream := f.getCurrentTrack().Stream.(*VideoStream)
		err = videoStream.Read(f.metaDataBuf)
		Fatal(err)
	}
	return nil
}

// GetType Получение типа текущего потока
func (stream *Stream) GetType() string {
	return stream.Type
}

// Read Чтение данные о потоке
func (stream *Stream) Read(buf *bytes.Reader) (err error) {
	defer Restore(&err, "ошибка чтения метаданных медиапотока")
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

// Read  Чтение информации об аудиопотоке
func (stream *AudioStream) Read(buf *bytes.Reader) (err error) {
	defer Restore(&err, "ошибка чтения метаданных аудиопотока")
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

// Read Чтение информации о видеопотоке
func (stream *VideoStream) Read(buf *bytes.Reader) (err error) {
	defer Restore(&err, "ошибка чтения метаданных видеопотока")
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
