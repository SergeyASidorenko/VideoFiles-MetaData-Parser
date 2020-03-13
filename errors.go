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

// Restore автовозврат ошибки
func Restore(err *error, msg string) {
	if err == nil || *err == nil {
		return
	}
	if r := recover(); r != nil {
		*err = r.(error)
		*err = NewAPIError(msg, *err)
	}
}

// Fatal автопаника при ошибке
func Fatal(err error) {
	if err != nil {
		panic(err)
	}
}

// APIError ошибка веб-сервиса
type APIError struct {
	APIMsg string
	Msg    string
	Err    error
}

// Error текст ошибки
func (e APIError) Error() string {
	var tempErr APIError
	err := e.Err
	msg := e.Msg
	for errors.Is(err, &tempErr) {
		msg = msg + "; " + tempErr.Error()
		err = tempErr.Err
	}
	msg += "; " + err.Error()
	return msg
}

// UnWrap извлечение ошибки
func (e APIError) UnWrap() error {
	return e.Err
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
	return APIError{APIMsg: msg, Msg: msg, Err: err}
}
