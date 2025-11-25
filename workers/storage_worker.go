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
	Dispatch ScanType = "DISPATCH"
)

type ScanInput struct {
	ScannerID uuid.UUID
	Type      ScanType
	Value     uuid.UUID
}

type ScanState struct {
	Pallet     uuid.UUID
	Location   uuid.UUID
	Dispatch   uuid.UUID
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

		st, exists := w.state[input.ScannerID]
		if !exists {
			st = &ScanState{LastUpdate: time.Now()}
			w.state[input.ScannerID] = st
		}

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
			st.Dispatch = uuid.Nil

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

		case Dispatch:
			st.Location = uuid.Nil

			if st.Dispatch != uuid.Nil && st.Dispatch != input.Value {
				logger.Log.Info("replacing previous dispatch with new",
					zap.String("scanner_id", input.ScannerID.String()),
					zap.String("old_dispatch_id", st.Dispatch.String()),
					zap.String("new_dispatch_id", input.Value.String()),
				)
			}
			st.Dispatch = input.Value
			st.LastUpdate = time.Now()
			logger.Log.Info("dispatch scanned",
				zap.String("scanner_id", input.ScannerID.String()),
				zap.String("dispatch_id", input.Value.String()),
			)
		}

		if st.Pallet != uuid.Nil && st.Location != uuid.Nil {
			logger.Log.Info("assigning pallet to location",
				zap.String("scanner_id", input.ScannerID.String()),
				zap.String("pallet_id", st.Pallet.String()),
				zap.String("location_id", st.Location.String()),
			)

			go func(scannerID, palletID, locationID uuid.UUID) {
				if err := storePalletToLocation(scannerID, palletID, locationID); err != nil {
					logger.Log.Error("failed to store pallet-location pair",
						zap.String("scanner_id", scannerID.String()),
						zap.Error(err))
				}
			}(input.ScannerID, st.Pallet, st.Location)

			w.clearState(input.ScannerID, st)
		} else if st.Pallet != uuid.Nil && st.Dispatch != uuid.Nil {
			logger.Log.Info("dispatching pallet",
				zap.String("scanner_id", input.ScannerID.String()),
				zap.String("pallet_id", st.Pallet.String()),
				zap.String("dispatch_id", st.Dispatch.String()),
			)

			go func(scannerID, palletID, dispatchID uuid.UUID) {
				if err := dispatchPallet(scannerID, palletID, dispatchID); err != nil {
					logger.Log.Error("failed to dispatch pallet",
						zap.String("scanner_id", scannerID.String()),
						zap.Error(err))
				}
			}(input.ScannerID, st.Pallet, st.Dispatch)

			w.clearState(input.ScannerID, st)
		}

		w.mu.Unlock()
	}
}

// Helper to clear state and stop timer
func (w *PairingWorker) clearState(scannerID uuid.UUID, st *ScanState) {
	if st.Timer != nil {
		st.Timer.Stop()
	}
	delete(w.state, scannerID)
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
				zap.String("dispatch_id", current.Dispatch.String()),
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
		time.Sleep(retryDelays[attempt-1])
	}
	return fmt.Errorf("pallet location failed to store after %d retries", maxRetries)
}

func dispatchPallet(scannerID, pallet, dispatch uuid.UUID) error {
	retryDelays := []time.Duration{5 * time.Second, 20 * time.Second, 1 * time.Minute}
	maxRetries := len(retryDelays)

	for attempt := 1; attempt <= maxRetries; attempt++ {
		_, err := api.Put[any](fmt.Sprintf("/pallets/shipped/%s", pallet), "")
		if err == nil {
			logger.Log.Info("successfully dispatched pallet",
				zap.String("scanner_id", scannerID.String()),
				zap.String("pallet_id", pallet.String()),
				zap.String("dispatch_id", dispatch.String()),
			)
			return nil
		}

		logger.Log.Warn("failed to dispatch pallet",
			zap.String("scanner_id", scannerID.String()),
			zap.Error(err),
			zap.Int("attempt", attempt),
		)

		if attempt < maxRetries {
			time.Sleep(retryDelays[attempt-1])
		}
	}
	return fmt.Errorf("failed to dispatch pallet after %d retries", maxRetries)
}
