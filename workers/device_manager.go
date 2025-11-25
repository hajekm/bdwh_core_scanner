package workers

import (
	"bdwh_core_scanner/api"
	"bdwh_core_scanner/logger"
	"bdwh_core_scanner/models"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/sstallion/go-hid"
	"go.uber.org/zap"
)

type ScannerManager struct {
	mu        sync.RWMutex
	scanners  []models.ScannerInfo
	interval  time.Duration
	stopChan  chan struct{}
	isRunning bool
}

func NewScannerManager(interval time.Duration) *ScannerManager {
	return &ScannerManager{
		interval: interval,
		stopChan: make(chan struct{}),
	}
}

func (m *ScannerManager) Start() {
	if m.isRunning {
		return
	}
	m.isRunning = true
	go m.worker()
}

func (m *ScannerManager) Stop() {
	if !m.isRunning {
		return
	}
	close(m.stopChan)
	m.isRunning = false
}

func (m *ScannerManager) worker() {
	ticker := time.NewTicker(m.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			m.refresh()
		case <-m.stopChan:
			return
		}
	}
}

func (m *ScannerManager) refresh() {
	if err := hid.Init(); err != nil {
		logger.Log.Error("hid.Init failed", zap.Error(err))
		return
	}
	defer func() {
		if err := hid.Exit(); err != nil {
			logger.Log.Error("hid.Exit failed", zap.Error(err))
		}
	}()

	var devices []models.ScannerInfo
	err := hid.Enumerate(0, 0, func(info *hid.DeviceInfo) error {
		if isLikelyScanner(info) {
			addOrAppendDevice(&devices, info)
		}
		return nil
	})
	if err != nil {
		logger.Log.Error("Enumerate failed", zap.Error(err))
		return
	}

	m.mu.Lock()
	m.scanners = devices
	m.mu.Unlock()

	m.syncWithAPI()
}

func (m *ScannerManager) GetScanners() []models.ScannerInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()
	cp := make([]models.ScannerInfo, len(m.scanners))
	copy(cp, m.scanners)
	return cp
}

func (m *ScannerManager) FindScannerIDByPath(path string) uuid.UUID {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, sc := range m.scanners {
		for _, p := range sc.Paths {
			if p == path {
				return sc.ID
			}
		}
	}
	return uuid.Nil
}

func (m *ScannerManager) syncWithAPI() {
	remote, err := api.Get[[]models.ScannerInfo]("/devices")
	if err != nil {
		logger.Log.Error("failed to fetch devices from API", zap.Error(err))
		return
	}
	logger.Log.Info("fetched devices from API", zap.Int("count", len(*remote)))

	m.mu.RLock()
	local := make([]models.ScannerInfo, len(m.scanners))
	copy(local, m.scanners)
	m.mu.RUnlock()

	if len(local) == 0 {
		logger.Log.Info("No local scanners found — skipping sync")
		return
	}

	// Build quick lookup maps
	localMap := make(map[string]models.ScannerInfo)
	for _, s := range local {
		localMap[s.ID.String()] = s
	}

	remoteMap := make(map[string]models.ScannerInfo)
	for _, s := range *remote {
		remoteMap[s.ID.String()] = s
	}

	for id, dev := range localMap {
		if _, found := remoteMap[id]; !found {
			logger.Log.Info("new device detected locally, sending POST", zap.String("id", id))
			if _, err := api.Post[models.ScannerInfo]("/devices", dev); err != nil {
				logger.Log.Error("failed to POST new device", zap.String("id", id), zap.Error(err))
			}
		}
	}

	// 🟥 Missing devices locally → DELETE from API
	for id := range remoteMap {
		if _, found := localMap[id]; !found {
			logger.Log.Info("Device removed locally, sending DELETE", zap.String("id", id))
			if err := api.Delete("/devices/" + id); err != nil {
				logger.Log.Error("Failed to DELETE device", zap.String("id", id), zap.Error(err))
			}
		}
	}
}

func addOrAppendDevice(list *[]models.ScannerInfo, d *hid.DeviceInfo) {
	for i := range *list {
		s := &(*list)[i]
		if isSameDevice(s, d) {
			if !contains(s.Paths, d.Path) {
				s.Paths = append(s.Paths, d.Path)
			}
			return
		}
	}

	*list = append(*list, models.ScannerInfo{
		ID:          generateStableID(d),
		VendorID:    d.VendorID,
		ProductID:   d.ProductID,
		BusType:     d.BusType.String(),
		ProductName: d.ProductStr,
		Serial:      d.SerialNbr,
		Paths:       []string{d.Path},
	})
}

func isSameDevice(s *models.ScannerInfo, d *hid.DeviceInfo) bool {
	if s.VendorID != d.VendorID || s.ProductID != d.ProductID {
		return false
	}
	if s.Serial == "" || d.SerialNbr == "" {
		return false
	}
	return s.Serial == d.SerialNbr
}

func contains(list []string, val string) bool {
	for _, v := range list {
		if v == val {
			return true
		}
	}
	return false
}

func generateStableID(d *hid.DeviceInfo) uuid.UUID {
	uniqueKey := d.SerialNbr
	if uniqueKey == "" {
		uniqueKey = d.Path
	}

	base := fmt.Sprintf("%04x:%04x:%s:%s",
		d.VendorID, d.ProductID, d.BusType.String(), uniqueKey)
	return uuid.NewSHA1(uuid.NameSpaceOID, []byte(base))
}

func isLikelyScanner(d *hid.DeviceInfo) bool {
	vid := d.VendorID
	pid := d.ProductID
	bus := d.BusType.String()
	name := strings.ToLower(d.ProductStr)

	if vid == 0x0581 && pid == 0x011a {
		return true
	}

	if bus == "USB" && vid == 0x0581 {
		return true
	}

	if strings.Contains(name, "barcode") ||
		strings.Contains(name, "scanner") ||
		strings.Contains(name, "honeywell") ||
		strings.Contains(name, "datalogic") ||
		strings.Contains(name, "zebra") {
		return true
	}

	if strings.Contains(name, "mouse") ||
		strings.Contains(name, "keyboard") ||
		strings.Contains(name, "touchpad") ||
		bus == "Bluetooth" || bus == "I2C" {
		return false
	}

	return false
}
