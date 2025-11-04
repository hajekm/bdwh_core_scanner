package workers

import (
	"bdwh_core_scanner/api"
	"bdwh_core_scanner/logger"
	"bdwh_core_scanner/models"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

type ScanType string

const (
	Pallet   ScanType = "PALLET"
	Location ScanType = "LOCATION"
)

type ScanInput struct {
	ScannerID uuid.UUID
	Type      ScanType
	Value     uuid.UUID
}

type ScanState struct {
	Pallet     uuid.UUID
	Location   uuid.UUID
	LastUpdate time.Time
	Timer      *time.Timer
}

type PairingWorker struct {
	inputChan chan ScanInput
	state     map[uuid.UUID]*ScanState
	mu        sync.Mutex
	timeout   time.Duration
}

func NewPairingWorker(timeout time.Duration) *PairingWorker {
	w := &PairingWorker{
		inputChan: make(chan ScanInput, 50),
		state:     make(map[uuid.UUID]*ScanState),
		timeout:   timeout,
	}
	go w.run()
	return w
}

func (w *PairingWorker) run() {
	for input := range w.inputChan {
		w.mu.Lock()

		// Get or create scanner-specific state
		st, exists := w.state[input.ScannerID]
		if !exists {
			st = &ScanState{LastUpdate: time.Now()}
			w.state[input.ScannerID] = st
		}

		// Reset timeout timer
		w.resetTimer(input.ScannerID, st)

		switch input.Type {
		case Pallet:
			if st.Pallet != uuid.Nil && st.Pallet != input.Value {
				logger.Log.Info("replacing previous pallet with new",
					zap.String("scanner_id", input.ScannerID.String()),
					zap.String("old_pallet_id", st.Pallet.String()),
					zap.String("new_pallet_id", input.Value.String()),
				)
			}
			st.Pallet = input.Value
			st.LastUpdate = time.Now()
			logger.Log.Info("pallet scanned",
				zap.String("scanner_id", input.ScannerID.String()),
				zap.String("pallet_id", input.Value.String()),
			)

		case Location:
			if st.Location != uuid.Nil && st.Location != input.Value {
				logger.Log.Info("replacing previous location with new",
					zap.String("scanner_id", input.ScannerID.String()),
					zap.String("old_location_id", st.Location.String()),
					zap.String("new_location_id", input.Value.String()),
				)
			}
			st.Location = input.Value
			st.LastUpdate = time.Now()
			logger.Log.Info("location scanned",
				zap.String("scanner_id", input.ScannerID.String()),
				zap.String("location_id", input.Value.String()),
			)
		}

		// 🟢 Check if both are now present for this scanner
		if st.Pallet != uuid.Nil && st.Location != uuid.Nil {
			logger.Log.Info("assigning pallet to location",
				zap.String("scanner_id", input.ScannerID.String()),
				zap.String("pallet_id", st.Pallet.String()),
				zap.String("location_id", st.Location.String()),
			)

			go func(scannerID uuid.UUID, palletID, locationID uuid.UUID) {
				if err := storePalletToLocation(scannerID, palletID, locationID); err != nil {
					logger.Log.Error("failed to store pallet-location pair",
						zap.String("scanner_id", scannerID.String()),
						zap.Error(err))
				}
			}(input.ScannerID, st.Pallet, st.Location)

			// Clean up after successful pairing
			if st.Timer != nil {
				st.Timer.Stop()
			}
			delete(w.state, input.ScannerID)
		}

		w.mu.Unlock()
	}
}

func (w *PairingWorker) resetTimer(scannerID uuid.UUID, st *ScanState) {
	if st.Timer != nil {
		st.Timer.Stop()
	}

	st.Timer = time.AfterFunc(w.timeout, func() {
		w.mu.Lock()
		defer w.mu.Unlock()

		if current, ok := w.state[scannerID]; ok {
			logger.Log.Info("timeout expired - removing incomplete scan",
				zap.String("scanner_id", scannerID.String()),
				zap.String("pallet_no", current.Pallet.String()),
				zap.String("location_code", current.Location.String()),
			)
			delete(w.state, scannerID)
		}
	})
}

func (w *PairingWorker) AddScan(scannerID uuid.UUID, scanType ScanType, value uuid.UUID) {
	w.inputChan <- ScanInput{
		ScannerID: scannerID,
		Type:      scanType,
		Value:     value,
	}
}

func storePalletToLocation(scannerID, pallet, location uuid.UUID) error {
	retryDelays := []time.Duration{
		5 * time.Second,
		20 * time.Second,
		1 * time.Minute,
		5 * time.Minute,
		20 * time.Minute,
		1 * time.Hour,
	}
	maxRetries := len(retryDelays)

	args := models.PalletLocation{
		ScannerID:  scannerID,
		PalletID:   pallet,
		LocationID: location,
	}

	for attempt := 1; attempt <= maxRetries; attempt++ {
		_, err := api.Post[models.PalletLocation]("/pallets/store", args)
		if err == nil {
			logger.Log.Info("stored pallet to location",
				zap.String("scanner_id", scannerID.String()),
				zap.String("pallet_id", pallet.String()),
				zap.String("location_id", location.String()),
				zap.Int("attempt", attempt),
			)
			return nil
		}

		logger.Log.Warn("failed to store pallet to location",
			zap.String("scanner_id", scannerID.String()),
			zap.String("pallet_id", pallet.String()),
			zap.String("location_id", location.String()),
			zap.Int("attempt", attempt),
			zap.Error(err),
		)

		if attempt == maxRetries {
			logger.Log.Error("max retries reached, giving up on storing pallet",
				zap.String("scanner_id", scannerID.String()),
				zap.String("pallet_id", pallet.String()),
				zap.String("location_id", location.String()),
			)
			return nil
		}

		delay := retryDelays[attempt-1]
		logger.Log.Info("retrying after delay",
			zap.String("scanner_id", scannerID.String()),
			zap.Duration("delay", delay),
			zap.Int("next_attempt", attempt+1),
		)
		time.Sleep(delay)
	}
	return fmt.Errorf("pallet location failed to store after %d retries", maxRetries)
}
