package workers

import (
	"bdwh_core_scanner/api"
	"bdwh_core_scanner/logger"
	"bdwh_core_scanner/models"
	"bdwh_core_scanner/utils"
	"bytes"
	"context"
	"strconv"
	"strings"
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

type QRWorker struct {
	scanChan       chan ScanMessage
	quitChan       chan struct{}
	wg             sync.WaitGroup
	activeScanners sync.Map
	manager        *ScannerManager
}

func NewQRWorker(interval time.Duration) *QRWorker {
	return &QRWorker{
		scanChan: make(chan ScanMessage, 50),
		quitChan: make(chan struct{}),
		manager:  NewScannerManager(interval),
	}
}

func (w *QRWorker) ListenForScans() {
	logger.Log.Info("Starting QRWorker HID monitor")

	ticker := time.NewTicker(w.manager.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			w.manager.refresh()

			current := w.manager.GetScanners()
			known := make(map[uuid.UUID]bool)

			for _, sc := range current {
				known[sc.ID] = true
				if _, exists := w.activeScanners.Load(sc.ID); !exists {
					for _, path := range sc.Paths {
						logger.Log.Info("Starting listener for scanner", zap.String("path", path))
						ctx, cancel := context.WithCancel(context.Background())
						w.activeScanners.Store(sc.ID, cancel)
						w.wg.Add(1)
						go w.listenDevice(ctx, path)
					}
				}
			}

			// Stop listeners for removed scanners
			w.activeScanners.Range(func(key, val any) bool {
				id := key.(uuid.UUID)
				cancel := val.(context.CancelFunc)
				if !known[id] {
					logger.Log.Info("Scanner removed, stopping listener", zap.String("id", id.String()))
					cancel()
					w.activeScanners.Delete(id)
				}
				return true
			})

		case <-w.quitChan:
			logger.Log.Info("Stopping QRWorker HID monitor")
			w.activeScanners.Range(func(_, val any) bool {
				val.(context.CancelFunc)()
				return true
			})
			w.wg.Wait()
			return
		}
	}
}

// 🔹 Listen to one /dev/hidrawX device
func (w *QRWorker) listenDevice(ctx context.Context, path string) {
	defer w.wg.Done()

	dev, err := hid.OpenPath(path)
	if err != nil {
		logger.Log.Error("Failed to open HID device", zap.String("path", path), zap.Error(err))
		return
	}
	defer dev.Close()

	buf := make([]byte, 8)
	var line bytes.Buffer
	var prevKey byte

	for {
		select {
		case <-ctx.Done():
			logger.Log.Info("Stopping HID listener", zap.String("path", path))
			return
		default:
		}

		n, err := dev.Read(buf)
		if err != nil {
			logger.Log.Warn("HID read error or disconnect", zap.String("path", path), zap.Error(err))
			return
		}
		if n < 3 {
			continue
		}

		mod := buf[0]
		key := buf[2]
		shift := mod&0x02 != 0

		if key == 0 || key == prevKey {
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
				scannerID := w.manager.FindScannerIDByPath(path)
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

func (w *QRWorker) ProcessScans(pairing *PairingWorker) {
	for {
		select {
		case msg := <-w.scanChan:
			scan := msg.Text
			scannerID := msg.ScannerID
			data := strings.Split(scan, "|")
			if len(data) == 0 {
				logger.Log.Info("QR worker received empty scan request")
				continue
			}
			if len(data) == 1 {
				p, err := api.Get[models.Pallet]("/pallets/code/" + data[0])
				if err != nil {
					ok := utils.IsValidLocationFormat(scan)
					if ok {
						l, err := api.Get[models.Location]("/locations/code/" + scan)
						if err != nil {
							logger.Log.Warn("QR worker received invalid scan request", zap.Error(err))
							continue
						}
						pairing.AddScan(scannerID, Location, l.ID)
					} else {
						logger.Log.Warn("QR worker received invalid scan request")
					}
					continue
				}
				pairing.AddScan(scannerID, Pallet, p.ID)
			} else {
				p, err := api.Get[models.Pallet]("/pallets/code/" + data[9])
				if err == nil {
					pairing.AddScan(scannerID, Pallet, p.ID)
				} else {
					bc, err := strconv.ParseInt(data[4], 10, 32)
					if err != nil {
						logger.Log.Info("QR worker received invalid scan request", zap.String("box_count", data[4]))
						continue
					}
					ipb, err := strconv.ParseInt(data[3], 10, 32)
					if err != nil {
						logger.Log.Info("QR worker received invalid scan request", zap.String("items_per_box", data[3]))
						continue
					}
					tc, err := strconv.ParseInt(data[5], 10, 32)
					if err != nil {
						logger.Log.Info("QR worker received invalid scan request", zap.String("total_count", data[5]))
						continue
					}
					if bc*ipb != tc {
						logger.Log.Warn("item counts do not match", zap.Int64("count", bc), zap.Int64("count", tc), zap.Int64("total_count", ipb), zap.Int64("items_per_box", ipb))
					}
					args := models.Pallet{
						BoxCount:            int32(bc),
						InvoiceNo:           data[0],
						ItemsPerBox:         int32(ipb),
						OriginShipToAddress: data[7],
						OriginShipToCountry: data[8],
						PalletNo:            data[9],
						PartNo:              data[1],
						SapNo:               data[2],
						TotalCount:          int32(tc),
						UnknownNo:           data[6],
					}
					p, err = api.Post[models.Pallet]("/pallets", args)
					if err != nil {
						logger.Log.Warn("QR worker received invalid scan request", zap.Error(err))
						continue
					}
					pairing.AddScan(scannerID, Pallet, p.ID)
				}
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
	w.activeScanners.Range(func(_, val any) bool {
		val.(context.CancelFunc)()
		return true
	})
	w.wg.Wait()
	close(w.scanChan)
}
