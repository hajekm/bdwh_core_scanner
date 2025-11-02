package main

import (
	"bdwh_core_scanner/api"
	"bdwh_core_scanner/logger"
	"bdwh_core_scanner/workers"
	"net/http"
	"os"
	"time"

	"go.uber.org/zap"
)

var baseUrl = ""

func main() {
	baseUrl = os.Getenv("BASE_URL")
	if baseUrl == "" {
		baseUrl = "http://192.168.0.201:8082/api/v1"
	} else {
		baseUrl = "https://" + baseUrl + "/api/v1"
	}
	api.Setup(baseUrl)
	logger.Log.Info("client initialized with base URL", zap.String("base_url", baseUrl))

	logger.Log.Info("starting scanner manager...")
	manager := workers.NewScannerManager(5 * time.Second)
	manager.Start()
	defer manager.Stop()
	logger.Log.Info("scanner manager started")

	pairing := workers.NewPairingWorker(1 * time.Minute)

	logger.Log.Info("starting qr worker...")
	worker := workers.NewQRWorker(5 * time.Second)
	go worker.ListenForScans()
	go worker.ProcessScans(pairing)
	defer worker.Stop()
	logger.Log.Info("qr worker started")

	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("OK"))
	})
	logger.Log.Info("starting local http server", zap.String("port", ":8080"))
	if err := http.ListenAndServe(":8080", nil); err != nil {
		logger.Log.Fatal("error starting http server", zap.Error(err))
	}
}
