// Copyright 2020 Sergey Sidorenko. All rights not reserved.
// Пакет с реализацией модудя извлечения метаинформации видеофайла в формате mp4
// Сведения о лицензии отсутствуют

// Функции работы с ошибками сервиса
package main

import (
	"errors"
	"fmt"
	"time"
)

// ErrFileIsNotValid ошибка - обрабатываемый файл не соответствует поддерживаемым форматам
var ErrFileIsNotValid = NewAPIError("формат файла неизвестен или не поддерживается", nil)

// ErrFileCodecNotSupported ошибка - обрабатываемый файл имеет неподдерживаемый алгоритм сжатия медиаданных
var ErrFileCodecNotSupported = NewAPIError("неподдерживаемый формат сжатия видеофайла", nil)

// restoreAndPanic автовозврат ошибки и снова вызов паники
func restoreAndPanic(msg string) {
	if r := recover(); r != nil {
		err := r.(error)
		panic(NewAPIError(msg, err))
	}
}

// restore автовозврат ошибки
func restore(err *error, msg string) {
	if err == nil || *err == nil {
		return
	}
	if r := recover(); r != nil {
		*err = r.(error)
		*err = NewAPIError(msg, *err)
	}
}

// fatal автопаника при ошибке
func fatal(err error) {
	if err != nil {
		panic(err)
	}
}

// APIError ошибка веб-сервиса
type APIError struct {
	APIMsg string
	msg    string
	err    error
}

// sysLog стек ошибок
func (e APIError) sysLog() string {
	var tempErr APIError
	err := e.err
	msg := e.msg
	for errors.As(err, &tempErr) {
		msg = msg + "; " + tempErr.Error()
		err = tempErr.err
	}
	// если объект внутренней ошибки существует - добавляем его содержимое
	if err != nil {
		msg += "; " + err.Error()
	}
	return msg
}

// Error текст ошибки
func (e APIError) Error() string {
	return e.APIMsg
}

// UnWrap извлечение ошибки
func (e APIError) UnWrap() error {
	return e.err
}

// MarshalJSON сериализация сведений об ошибке в формате JSON
func (e APIError) MarshalJSON() (b []byte, err error) {
	s := fmt.Sprintf("{\"%s\":\"%s\",\"%s\":\"%s\"}",
		"Error",
		e.APIMsg,
		"Time",
		time.Now().Format(time.RFC822))
	return []byte(s), nil
}

// NewAPIError создание новой ошибки
func NewAPIError(msg string, err error) (e APIError) {
	return APIError{APIMsg: msg, msg: msg, err: err}
}
