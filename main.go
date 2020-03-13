// Copyright 2020 The Home. All rights not reserved.
// Пакет с реализацией модудя извлечения метаинформации видеофайла в формате mp4
// Сведения о лицензии отсутствуют

// Получение содержимого видеофайла в теле HTTP POST запроса и возврата метаданных файла в формате JSON
package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
)

// инициализования лога для ошибок
func initLog(filePath string) error {
	var logFile *os.File
	if _, err := os.Stat(filePath); err != nil {
		if os.IsNotExist(err) {
			logFile, err = os.Create(filePath)
			if err != nil {
				return fmt.Errorf("не удалось инициализировать логирование ошибок: %w", err)
			}
		} else {
			return fmt.Errorf("не удалось инициализировать логирование ошибок: %w", err)
		}
	} else {
		logFile, err = os.OpenFile(filePath, os.O_APPEND|os.O_WRONLY, 0600)
		if err != nil {
			return fmt.Errorf("не удалось инициализировать логирование ошибок: %w", err)
		}
	}
	// сопоставляем созданный файл, как приемник логирования
	log.SetOutput(logFile)
	return nil
}
func parseVideoInForm(res http.ResponseWriter, req *http.Request) {
	if req.Method == "POST" {
		var fileInfo VideoFile
		defer func() {
			if r := recover(); r != nil {
				e := r.(error)
				apiErr := fileInfo.GetError(e)
				log.Printf(apiErr.Error())
				data, _ := json.Marshal(apiErr)
				res.Write(data)
			}
		}()
		var data []byte
		res.Header().Set("Content-Type", "text/json")
		err := fileInfo.Open(req.Body)
		Fatal(err)
		defer req.Body.Close()
		err = fileInfo.Parse()
		Fatal(err)
		data, err = fileInfo.ToJSON()
		Fatal(err)
		res.Write(data)
	} else {
		res.WriteHeader(http.StatusBadRequest)
	}
}

func main() {
	err := initLog("logs/errors.log")
	if err != nil {
		log.Fatalln(err)
	}
	http.HandleFunc("/api/mp4Meta", parseVideoInForm)
	http.ListenAndServe(":4000", nil)
}
