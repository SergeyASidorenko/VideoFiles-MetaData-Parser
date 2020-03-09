// Copyright 2020 The Home. All rights not reserved.
// Пакет с реализацией тестового задания
// Сведения о лицензии отсутствуют

// Функции работы с ошибками сервиса
package main

import (
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
		var e = NewAPIError(msg, *err)
		*err = e
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
	return e.Msg + " " + e.Err.Error()
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
	e = APIError{APIMsg: msg, Msg: msg, Err: err}
	return e
}
