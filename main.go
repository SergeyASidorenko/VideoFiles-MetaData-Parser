// Copyright 2020 The Home. All rights not reserved.
// Пакет с реализацией тестового задания
// Сведения о лицензии отсутствуют

// Получение метаинформации о видеопотоке/видеофайле
package main

import (
	"log"
	"net/http"
	"os"
)

func initLog(filePath string) error {
	var logFile *os.File
	if _, err := os.Stat(filePath); err != nil {
		if os.IsNotExist(err) {
			logFile, err = os.Create(filePath)
		}
	} else {
		logFile, err = os.OpenFile(filePath, os.O_APPEND|os.O_WRONLY, 0600)
		if err != nil {
			log.Fatalln("не удалось инициализировать логирование ошибок")
		}
	}
	log.SetOutput(logFile)
	return nil
}
func parseVideoInForm(res http.ResponseWriter, req *http.Request) {
	if req.Method == "POST" {
		var fileInfo VideoFile
		defer (func() {
			if r := recover(); r != nil {
				e := r.(error)
				data, _ := fileInfo.SendError(e)
				res.Write(data)
			}
		})()
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
