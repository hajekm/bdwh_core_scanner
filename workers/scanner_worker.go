package workers

import (
	"bdwh_core_scanner/api"
	"bdwh_core_scanner/logger"
	"bdwh_core_scanner/models"
	"bytes"
	"context"
	"path/filepath"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/sstallion/go-hid"
	"go.uber.org/zap"
)

type ScanMessage struct {
	ScannerID uuid.UUID
	Text      string
}

type ScanEvent struct {
	ScannerID uuid.UUID `json:"scanner_id"`
	Text      string    `json:"text"`
	Timestamp time.Time `json:"timestamp"`
}

type QRWorker struct {
	scanChan       chan ScanMessage
	quitChan       chan struct{}
	wg             sync.WaitGroup
	activeScanners sync.Map
	manager        *ScannerManager
}

func NewQRWorker(manager *ScannerManager) *QRWorker {
	return &QRWorker{
		scanChan: make(chan ScanMessage, 50),
		quitChan: make(chan struct{}),
		manager:  manager,
	}
}

func (w *QRWorker) ListenForScans() {
	logger.Log.Info("Starting QRWorker with FIXED paths")
	fixedPaths := []string{"/dev/scanner_top", "/dev/scanner_bottom"}

	for _, path := range fixedPaths {
		logger.Log.Info("Starting listener for fixed scanner", zap.String("path", path))

		ctx, cancel := context.WithCancel(context.Background())
		realPath, err := filepath.EvalSymlinks(path)
		var scannerID uuid.UUID

		if err == nil {
			scannerID = w.manager.FindScannerIDByPath(realPath)
		}
		if scannerID == uuid.Nil {
			logger.Log.Warn("Could not determine ID for path (manager has no record yet)", zap.String("path", path))
			scannerID = generateIDFromPath(path)
		}
		w.activeScanners.Store(scannerID, cancel)

		w.wg.Add(1)
		go w.listenDevice(ctx, path, scannerID)
	}

	<-w.quitChan

	logger.Log.Info("Stopping QRWorker HID monitor")
	w.activeScanners.Range(func(_, val any) bool {
		val.(context.CancelFunc)()
		return true
	})
	w.wg.Wait()
}

func (w *QRWorker) listenDevice(ctx context.Context, path string, initID uuid.UUID) {
	defer w.wg.Done()

	for {
		select {
		case <-ctx.Done():
			return
		default:
			dev, err := hid.OpenPath(path)
			if err != nil {
				logger.Log.Warn("Failed to open HID device, retrying in 2s...", zap.String("path", path), zap.Error(err))
				select {
				case <-time.After(2 * time.Second):
					continue
				case <-ctx.Done():
					return
				}
			}

			logger.Log.Info("HID device connected", zap.String("path", path))
			scannerID := initID
			realPath, _ := filepath.EvalSymlinks(path)

			for i := 0; i < 50; i++ {
				foundID := w.manager.FindScannerIDByPath(realPath)
				if foundID != uuid.Nil {
					scannerID = foundID
					break
				}
				time.Sleep(100 * time.Millisecond)
			}

			if scannerID == uuid.Nil {
				logger.Log.Error("Manager still doesn't know this device! Scans might be rejected.", zap.String("path", path), zap.String("real_path", realPath))
			} else {
				logger.Log.Info("Scanner identified successfully", zap.String("path", path), zap.String("id", scannerID.String()))
			}
			w.readLoop(ctx, dev, path, scannerID)

			err = dev.Close()
			if err != nil {
				logger.Log.Warn("Failed to close HID device", zap.String("path", path), zap.Error(err))
			}
			logger.Log.Info("HID device disconnected, attempting reconnect...", zap.String("path", path))
			time.Sleep(1 * time.Second)
		}
	}
}

func (w *QRWorker) readLoop(ctx context.Context, dev *hid.Device, path string, scannerID uuid.UUID) {
	buf := make([]byte, 16)
	var line bytes.Buffer
	var prevKey byte

	for {
		select {
		case <-ctx.Done():
			return
		default:
			n, err := dev.Read(buf)
			if err != nil {
				logger.Log.Warn("HID read error (device disconnected?)", zap.String("path", path), zap.Error(err))
				return
			}
			if n < 3 {
				continue
			}

			mod := buf[0]
			key := buf[2]
			shift := mod&0x02 != 0

			if key == 0 || (key == prevKey && key != 0) {
				if key == 0 {
					prevKey = key
					continue
				}
				prevKey = key
				continue
			}
			prevKey = key

			char := decodeHIDKey(key, shift)
			if char == "" {
				continue
			}

			if char == "\n" {
				text := line.String()
				line.Reset()
				if text != "" {
					select {
					case w.scanChan <- ScanMessage{ScannerID: scannerID, Text: text}:
						logger.Log.Info("Scanned QR", zap.String("path", path), zap.String("code", text))
					case <-ctx.Done():
						return
					}
				}
			} else {
				line.WriteString(char)
			}
		}
	}
}

func decodeHIDKey(code byte, shift bool) string {
	hidMap := map[byte][2]string{
		4: {"a", "A"}, 5: {"b", "B"}, 6: {"c", "C"}, 7: {"d", "D"}, 8: {"e", "E"},
		9: {"f", "F"}, 10: {"g", "G"}, 11: {"h", "H"}, 12: {"i", "I"}, 13: {"j", "J"},
		14: {"k", "K"}, 15: {"l", "L"}, 16: {"m", "M"}, 17: {"n", "N"}, 18: {"o", "O"},
		19: {"p", "P"}, 20: {"q", "Q"}, 21: {"r", "R"}, 22: {"s", "S"}, 23: {"t", "T"},
		24: {"u", "U"}, 25: {"v", "V"}, 26: {"w", "W"}, 27: {"x", "X"}, 28: {"y", "Y"},
		29: {"z", "Z"},
		30: {"1", "!"}, 31: {"2", "@"}, 32: {"3", "#"}, 33: {"4", "$"}, 34: {"5", "%"},
		35: {"6", "^"}, 36: {"7", "&"}, 37: {"8", "*"}, 38: {"9", "("}, 39: {"0", ")"},
		40: {"\n", "\n"},
		44: {" ", " "}, 45: {"-", "_"}, 46: {"=", "+"}, 47: {"[", "{"}, 48: {"]", "}"},
		49: {"\\", "|"}, 51: {";", ":"}, 52: {"'", "\""}, 53: {"`", "~"},
		54: {",", "<"}, 55: {".", ">"}, 56: {"/", "?"},
	}

	if val, ok := hidMap[code]; ok {
		if shift {
			return val[1]
		}
		return val[0]
	}
	return ""
}

func (w *QRWorker) ProcessScans() {
	for {
		select {
		case msg := <-w.scanChan:
			event := ScanEvent{
				ScannerID: msg.ScannerID,
				Text:      msg.Text,
				Timestamp: time.Now(),
			}
			_, err := api.Post[any]("/scans", event)
			if err != nil {
				logger.Log.Error("Failed to send scan to server",
					zap.String("scanner_id", msg.ScannerID.String()),
					zap.Error(err))
			} else {
				logger.Log.Info("Scan sent to server",
					zap.String("scanner_id", msg.ScannerID.String()))
			}
		case <-w.quitChan:
			logger.Log.Info("Stopping QR processor")
			return
		}
	}
}

func (w *QRWorker) GetScanners() []models.ScannerInfo {
	return w.manager.GetScanners()
}

func (w *QRWorker) Stop() {
	close(w.quitChan)
	close(w.scanChan)
}

func generateIDFromPath(path string) uuid.UUID {
	return uuid.NewSHA1(uuid.NameSpaceOID, []byte(path))
}
