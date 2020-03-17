// Copyright 2020 Sergey Sidorenko. All rights not reserved.
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
		var data []byte
		res.Header().Set("Content-Type", "text/json")
		err := fileInfo.Open(req.Body)
		if err != nil {
			sendError(res, err)
			return
		}
		defer req.Body.Close()
		err = fileInfo.Parse()
		if err != nil {
			sendError(res, err)
			return
		}
		data, err = fileInfo.ToJSON()
		if err != nil {
			sendError(res, err)
			return
		}
		res.Write(data)
	} else {
		res.WriteHeader(http.StatusBadRequest)
	}
}
func sendError(w http.ResponseWriter, e error) {
	log.Printf(e.Error())
	data, err := json.Marshal(e)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
	} else {
		w.Write(data)
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
