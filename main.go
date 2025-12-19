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
		baseUrl = "http://192.168.0.156:8082/api/v1"
	} else {
		baseUrl = baseUrl + "/api/v1"
	}
	token := os.Getenv("API_HASH")
	if token == "" {
		logger.Log.Fatal("API_HASH environment variable not set")
	}
	duration := os.Getenv("DURATION")
	refreshDuration := 5 * time.Second
	if duration != "" {
		d, err := time.ParseDuration(duration)
		if err == nil {
			logger.Log.Info("refreshing scanners every", zap.Duration("duration", d))
			refreshDuration = d
		}
	}
	api.Setup(baseUrl, token)
	logger.Log.Info("client initialized with base URL", zap.String("base_url", baseUrl))

	logger.Log.Info("starting scanner manager...")
	manager := workers.NewScannerManager(refreshDuration)
	manager.Start()
	defer manager.Stop()
	logger.Log.Info("scanner manager started")

	logger.Log.Info("starting qr worker...")
	worker := workers.NewQRWorker(manager)
	go worker.ListenForScans()
	go worker.ProcessScans()
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
