package service_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/SavaArsikin/ubiquiti-device-monitoring-service/internal/device"
	"github.com/SavaArsikin/ubiquiti-device-monitoring-service/internal/devicestatus"
	"github.com/SavaArsikin/ubiquiti-device-monitoring-service/internal/service"
	"github.com/google/go-cmp/cmp"
)

type deviceRepositoryStub struct {
	result     *device.Device
	listResult []*device.Device
	err        error

	calls  int
	method string
	input  *device.Device
	id     string
	tx     *sql.Tx
	ctx    context.Context
}

func (s *deviceRepositoryStub) Create(
	ctx context.Context,
	input *device.Device,
	tx *sql.Tx,
) (*device.Device, error) {
	s.calls++
	s.method = "Create"
	s.ctx = ctx
	s.tx = tx
	s.input = nil
	if input != nil {
		copied := *input
		s.input = &copied
	}

	return s.result, s.err
}

func (s *deviceRepositoryStub) Get(
	ctx context.Context,
	id string,
	tx *sql.Tx,
) (*device.Device, error) {
	s.calls++
	s.method = "Get"
	s.ctx = ctx
	s.id = id
	s.tx = tx

	return s.result, s.err
}

func (s *deviceRepositoryStub) ListActive(
	ctx context.Context,
	tx *sql.Tx,
) ([]*device.Device, error) {
	s.calls++
	s.method = "ListActive"
	s.ctx = ctx
	s.tx = tx

	return s.listResult, s.err
}

func (s *deviceRepositoryStub) ListArchived(
	ctx context.Context,
	tx *sql.Tx,
) ([]*device.Device, error) {
	s.calls++
	s.method = "ListArchived"
	s.ctx = ctx
	s.tx = tx

	return s.listResult, s.err
}

func (s *deviceRepositoryStub) Update(
	ctx context.Context,
	input *device.Device,
	tx *sql.Tx,
) (*device.Device, error) {
	s.calls++
	s.method = "Update"
	s.ctx = ctx
	s.tx = tx
	s.input = nil
	if input != nil {
		copied := *input
		s.input = &copied
	}

	return s.result, s.err
}

func (s *deviceRepositoryStub) Archive(
	ctx context.Context,
	id string,
	tx *sql.Tx,
) error {
	s.calls++
	s.method = "Archive"
	s.ctx = ctx
	s.id = id
	s.tx = tx

	return s.err
}

func (s *deviceRepositoryStub) Restore(
	ctx context.Context,
	id string,
	tx *sql.Tx,
) (*device.Device, error) {
	s.calls++
	s.method = "Restore"
	s.ctx = ctx
	s.id = id
	s.tx = tx

	return s.result, s.err
}

type statusRepositoryStub struct {
	result *devicestatus.DeviceStatus
	err    error

	calls int
	input *devicestatus.DeviceStatus
	tx    *sql.Tx
}

func (s *statusRepositoryStub) Create(
	ctx context.Context,
	input *devicestatus.DeviceStatus,
	tx *sql.Tx,
) (*devicestatus.DeviceStatus, error) {
	s.calls++
	s.input = nil
	if input != nil {
		copied := *input
		s.input = &copied
	}
	s.tx = tx

	return s.result, s.err
}

func openMockDatabase(t *testing.T) (*sql.DB, sqlmock.Sqlmock) {
	t.Helper()

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create mock database: %v", err)
	}

	t.Cleanup(func() {
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("transaction expectations: %v", err)
		}

		mock.ExpectClose()
		if err := db.Close(); err != nil {
			t.Errorf("close mock database: %v", err)
		}
	})

	return db, mock
}

func TestCreateDevice(t *testing.T) {
	t.Run("creates device and initial status then commits", func(t *testing.T) {
		db, mock := openMockDatabase(t)
		mock.ExpectBegin()
		mock.ExpectCommit()

		input := device.Device{
			Name:      " Router ",
			HealthURL: " http://localhost:8081/health ",
		}
		originalInput := input

		expectedInput := device.Device{
			Name:      "Router",
			HealthURL: "http://localhost:8081/health",
		}
		saved := expectedInput
		saved.ID = "device-1"

		initialStatus := devicestatus.DeviceStatus{
			DeviceID:           saved.ID,
			ReachabilityStatus: devicestatus.ReachabilityUnknown,
		}

		devices := &deviceRepositoryStub{result: &saved}
		statuses := &statusRepositoryStub{result: &initialStatus}
		deviceService := service.NewDeviceService(db, devices, statuses)

		created, err := deviceService.Create(t.Context(), &input)
		if err != nil {
			t.Fatalf("create device: %v", err)
		}

		if devices.calls != 1 || statuses.calls != 1 {
			t.Fatalf("expected one call each, got devices=%d statuses=%d",
				devices.calls, statuses.calls)
		}
		if devices.tx == nil || devices.tx != statuses.tx {
			t.Error("expected both repositories to use the same transaction")
		}
		if diff := cmp.Diff(&expectedInput, devices.input); diff != "" {
			t.Errorf("device input mismatch (-want +got):\n%s", diff)
		}
		if diff := cmp.Diff(&initialStatus, statuses.input); diff != "" {
			t.Errorf("status input mismatch (-want +got):\n%s", diff)
		}
		if diff := cmp.Diff(&saved, created); diff != "" {
			t.Errorf("returned device mismatch (-want +got):\n%s", diff)
		}
		if diff := cmp.Diff(originalInput, input); diff != "" {
			t.Errorf("caller input changed (-before +after):\n%s", diff)
		}
	})

	for _, stage := range []string{"device", "status"} {
		t.Run("rolls back on "+stage+" creation failure", func(t *testing.T) {
			db, mock := openMockDatabase(t)
			mock.ExpectBegin()
			mock.ExpectRollback()

			expectedErr := errors.New("repository failure")
			input := device.Device{
				Name:      "Router",
				HealthURL: "http://localhost:8081/health",
			}
			saved := input
			saved.ID = "device-1"

			devices := &deviceRepositoryStub{result: &saved}
			statuses := &statusRepositoryStub{}

			expectedStatusCalls := 1
			if stage == "device" {
				devices.result = nil
				devices.err = expectedErr
				expectedStatusCalls = 0
			} else {
				statuses.err = expectedErr
			}

			deviceService := service.NewDeviceService(db, devices, statuses)
			created, err := deviceService.Create(t.Context(), &input)
			if !errors.Is(err, expectedErr) {
				t.Fatalf("expected repository failure, got: %v", err)
			}
			if created != nil {
				t.Errorf("expected nil device, got %+v", created)
			}
			if devices.calls != 1 || statuses.calls != expectedStatusCalls {
				t.Fatalf("unexpected repository calls: devices=%d statuses=%d",
					devices.calls, statuses.calls)
			}
			if devices.tx == nil {
				t.Error("expected device creation to use a transaction")
			}
			if stage == "status" && statuses.tx != devices.tx {
				t.Error("expected both repositories to use the same transaction")
			}
		})
	}

	for _, tc := range []struct {
		name    string
		input   *device.Device
		wantErr string
	}{
		{
			name:    "nil input",
			wantErr: "device is required",
		},
		{
			name: "blank name",
			input: &device.Device{
				Name:      " ",
				HealthURL: "http://localhost:8081/health",
			},
			wantErr: "device name is required",
		},
		{
			name: "invalid health URL",
			input: &device.Device{
				Name:      "Router",
				HealthURL: "not-a-url",
			},
			wantErr: "health URL must be an HTTP or HTTPS URL with a host",
		},
	} {
		t.Run("rejects "+tc.name, func(t *testing.T) {
			db, _ := openMockDatabase(t)

			devices := &deviceRepositoryStub{}
			statuses := &statusRepositoryStub{}
			deviceService := service.NewDeviceService(db, devices, statuses)

			created, err := deviceService.Create(t.Context(), tc.input)
			if err == nil || err.Error() != tc.wantErr {
				t.Fatalf("expected error %q, got: %v", tc.wantErr, err)
			}
			if created != nil {
				t.Errorf("expected nil device, got %+v", created)
			}
			if devices.calls != 0 || statuses.calls != 0 {
				t.Errorf("expected no repository calls, got devices=%d statuses=%d",
					devices.calls, statuses.calls)
			}
		})
	}
}

const testDeviceID = "11111111-1111-4111-8111-111111111111"

func TestGetDevice(t *testing.T) {
	testCases := []struct {
		name          string
		id            string
		repositoryErr error
		validationErr string
	}{
		{
			name: "success",
			id:   testDeviceID,
		},
		{
			name:          "repository error",
			id:            testDeviceID,
			repositoryErr: device.ErrDeviceNotFound,
		},
		{
			name:          "invalid ID",
			id:            "not-a-uuid",
			validationErr: "device ID must be a valid UUID",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			db, _ := openMockDatabase(t)

			expected := &device.Device{
				ID:        testDeviceID,
				Name:      "Router",
				HealthURL: "http://localhost:8081/health",
			}

			devices := &deviceRepositoryStub{
				result: expected,
				err:    testCase.repositoryErr,
			}
			statuses := &statusRepositoryStub{}
			deviceService := service.NewDeviceService(db, devices, statuses)

			ctx := t.Context()
			got, err := deviceService.Get(ctx, testCase.id)

			if testCase.validationErr != "" {
				if err == nil || err.Error() != testCase.validationErr {
					t.Fatalf("expected error %q, got: %v", testCase.validationErr, err)
				}
				if got != nil || devices.calls != 0 {
					t.Fatal("expected nil result and no repository calls")
				}
			} else {
				if devices.calls != 1 || devices.method != "Get" {
					t.Fatalf("expected one Get call, got %d %s calls",
						devices.calls, devices.method)
				}
				if devices.id != testCase.id || devices.tx != nil || devices.ctx != ctx {
					t.Error("unexpected repository arguments")
				}

				if testCase.repositoryErr != nil {
					if !errors.Is(err, testCase.repositoryErr) {
						t.Fatalf("expected repository error, got: %v", err)
					}
					if got != nil {
						t.Errorf("expected nil device, got %+v", got)
					}
				} else {
					if err != nil {
						t.Fatalf("get device: %v", err)
					}
					if diff := cmp.Diff(expected, got); diff != "" {
						t.Errorf("device mismatch (-want +got):\n%s", diff)
					}
				}
			}

			if statuses.calls != 0 {
				t.Error("status repository should not be called")
			}
		})
	}
}

func TestRestoreDevice(t *testing.T) {
	testCases := []struct {
		name          string
		id            string
		repositoryErr error
		validationErr string
	}{
		{
			name: "success",
			id:   testDeviceID,
		},
		{
			name:          "repository error",
			id:            testDeviceID,
			repositoryErr: device.ErrDeviceNotFound,
		},
		{
			name:          "invalid ID",
			id:            "not-a-uuid",
			validationErr: "device ID must be a valid UUID",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			db, _ := openMockDatabase(t)

			expected := &device.Device{
				ID:        testDeviceID,
				Name:      "Router",
				HealthURL: "http://localhost:8081/health",
			}

			devices := &deviceRepositoryStub{
				result: expected,
				err:    testCase.repositoryErr,
			}
			statuses := &statusRepositoryStub{}
			deviceService := service.NewDeviceService(db, devices, statuses)

			ctx := t.Context()
			got, err := deviceService.Restore(ctx, testCase.id)

			if testCase.validationErr != "" {
				if err == nil || err.Error() != testCase.validationErr {
					t.Fatalf("expected error %q, got: %v", testCase.validationErr, err)
				}
				if got != nil || devices.calls != 0 {
					t.Fatal("expected nil result and no repository calls")
				}
			} else {
				if devices.calls != 1 || devices.method != "Restore" {
					t.Fatalf("expected one Restore call, got %d %s calls",
						devices.calls, devices.method)
				}
				if devices.id != testCase.id || devices.tx != nil || devices.ctx != ctx {
					t.Error("unexpected repository arguments")
				}

				if testCase.repositoryErr != nil {
					if !errors.Is(err, testCase.repositoryErr) {
						t.Fatalf("expected repository error, got: %v", err)
					}
					if got != nil {
						t.Errorf("expected nil device, got %+v", got)
					}
				} else {
					if err != nil {
						t.Fatalf("restore device: %v", err)
					}
					if diff := cmp.Diff(expected, got); diff != "" {
						t.Errorf("device mismatch (-want +got):\n%s", diff)
					}
				}
			}

			if statuses.calls != 0 {
				t.Error("status repository should not be called")
			}
		})
	}
}

func TestListActiveDevices(t *testing.T) {
	repositoryErr := errors.New("list failed")

	testCases := []struct {
		name    string
		result  []*device.Device
		wantErr error
	}{
		{
			name: "success",
			result: []*device.Device{
				{
					ID:        testDeviceID,
					Name:      "Router",
					HealthURL: "http://localhost:8081/health",
				},
			},
		},
		{
			name:   "empty list",
			result: []*device.Device{},
		},
		{
			name:    "repository error",
			wantErr: repositoryErr,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			db, _ := openMockDatabase(t)
			devices := &deviceRepositoryStub{
				listResult: testCase.result,
				err:        testCase.wantErr,
			}
			statuses := &statusRepositoryStub{}
			deviceService := service.NewDeviceService(db, devices, statuses)

			ctx := t.Context()
			got, err := deviceService.ListActive(ctx)

			if devices.calls != 1 || devices.method != "ListActive" {
				t.Fatalf("expected one ListActive call, got %d %s calls",
					devices.calls, devices.method)
			}
			if devices.tx != nil || devices.ctx != ctx {
				t.Error("unexpected repository arguments")
			}
			if statuses.calls != 0 {
				t.Error("status repository should not be called")
			}

			if testCase.wantErr != nil {
				if !errors.Is(err, testCase.wantErr) {
					t.Fatalf("expected repository error, got: %v", err)
				}
				if got != nil {
					t.Errorf("expected nil list, got %+v", got)
				}
			} else {
				if err != nil {
					t.Fatalf("list active devices: %v", err)
				}
				if diff := cmp.Diff(testCase.result, got); diff != "" {
					t.Errorf("device list mismatch (-want +got):\n%s", diff)
				}
			}
		})
	}
}

func TestListArchivedDevices(t *testing.T) {
	repositoryErr := errors.New("list failed")

	testCases := []struct {
		name    string
		result  []*device.Device
		wantErr error
	}{
		{
			name: "success",
			result: []*device.Device{
				{
					ID:        testDeviceID,
					Name:      "Router",
					HealthURL: "http://localhost:8081/health",
				},
			},
		},
		{
			name:   "empty list",
			result: []*device.Device{},
		},
		{
			name:    "repository error",
			wantErr: repositoryErr,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			db, _ := openMockDatabase(t)
			devices := &deviceRepositoryStub{
				listResult: testCase.result,
				err:        testCase.wantErr,
			}
			statuses := &statusRepositoryStub{}
			deviceService := service.NewDeviceService(db, devices, statuses)

			ctx := t.Context()
			got, err := deviceService.ListArchived(ctx)

			if devices.calls != 1 || devices.method != "ListArchived" {
				t.Fatalf("expected one ListArchived call, got %d %s calls",
					devices.calls, devices.method)
			}
			if devices.tx != nil || devices.ctx != ctx {
				t.Error("unexpected repository arguments")
			}
			if statuses.calls != 0 {
				t.Error("status repository should not be called")
			}

			if testCase.wantErr != nil {
				if !errors.Is(err, testCase.wantErr) {
					t.Fatalf("expected repository error, got: %v", err)
				}
				if got != nil {
					t.Errorf("expected nil list, got %+v", got)
				}
			} else {
				if err != nil {
					t.Fatalf("list archived devices: %v", err)
				}
				if diff := cmp.Diff(testCase.result, got); diff != "" {
					t.Errorf("device list mismatch (-want +got):\n%s", diff)
				}
			}
		})
	}
}

func TestUpdateDevice(t *testing.T) {
	t.Run("validates and updates without changing caller input", func(t *testing.T) {
		db, _ := openMockDatabase(t)

		input := device.Device{
			ID:        testDeviceID,
			Name:      " Updated router ",
			HealthURL: " http://localhost:8082/health ",
		}
		original := input
		expected := device.Device{
			ID:        testDeviceID,
			Name:      "Updated router",
			HealthURL: "http://localhost:8082/health",
		}

		devices := &deviceRepositoryStub{result: &expected}
		statuses := &statusRepositoryStub{}
		deviceService := service.NewDeviceService(db, devices, statuses)

		ctx := t.Context()
		got, err := deviceService.Update(ctx, &input)
		if err != nil {
			t.Fatalf("update device: %v", err)
		}

		if devices.calls != 1 || devices.method != "Update" {
			t.Fatal("expected one Update call")
		}
		if devices.tx != nil || devices.ctx != ctx {
			t.Error("unexpected repository arguments")
		}
		if statuses.calls != 0 {
			t.Error("status repository should not be called")
		}
		if diff := cmp.Diff(&expected, devices.input); diff != "" {
			t.Errorf("repository input mismatch (-want +got):\n%s", diff)
		}
		if diff := cmp.Diff(&expected, got); diff != "" {
			t.Errorf("returned device mismatch (-want +got):\n%s", diff)
		}
		if diff := cmp.Diff(original, input); diff != "" {
			t.Errorf("caller input changed (-before +after):\n%s", diff)
		}
	})

	t.Run("propagates repository error", func(t *testing.T) {
		db, _ := openMockDatabase(t)
		devices := &deviceRepositoryStub{err: device.ErrDeviceNotFound}
		statuses := &statusRepositoryStub{}
		deviceService := service.NewDeviceService(db, devices, statuses)

		input := device.Device{
			ID:        testDeviceID,
			Name:      "Router",
			HealthURL: "http://localhost:8081/health",
		}
		got, err := deviceService.Update(t.Context(), &input)
		if !errors.Is(err, device.ErrDeviceNotFound) {
			t.Fatalf("expected device not found, got: %v", err)
		}
		if got != nil {
			t.Errorf("expected nil device, got %+v", got)
		}
		if devices.calls != 1 || devices.method != "Update" || statuses.calls != 0 {
			t.Error("unexpected repository calls")
		}
	})

	for _, tc := range []struct {
		name    string
		input   *device.Device
		wantErr string
	}{
		{
			name:    "nil input",
			wantErr: "device is required",
		},
		{
			name: "blank name",
			input: &device.Device{
				ID:        testDeviceID,
				Name:      " ",
				HealthURL: "http://localhost:8081/health",
			},
			wantErr: "device name is required",
		},
		{
			name: "invalid URL",
			input: &device.Device{
				ID:        testDeviceID,
				Name:      "Router",
				HealthURL: "not-a-url",
			},
			wantErr: "health URL must be an HTTP or HTTPS URL with a host",
		},
		{
			name: "invalid ID",
			input: &device.Device{
				ID:        "not-a-uuid",
				Name:      "Router",
				HealthURL: "http://localhost:8081/health",
			},
			wantErr: "device ID must be a valid UUID",
		},
	} {
		t.Run("rejects "+tc.name, func(t *testing.T) {
			db, _ := openMockDatabase(t)
			devices := &deviceRepositoryStub{}
			statuses := &statusRepositoryStub{}
			deviceService := service.NewDeviceService(db, devices, statuses)

			got, err := deviceService.Update(t.Context(), tc.input)
			if err == nil || err.Error() != tc.wantErr {
				t.Fatalf("expected error %q, got: %v", tc.wantErr, err)
			}
			if got != nil {
				t.Errorf("expected nil device, got %+v", got)
			}
			if devices.calls != 0 || statuses.calls != 0 {
				t.Error("repositories should not be called")
			}
		})
	}
}

func TestArchiveDevice(t *testing.T) {
	for _, tc := range []struct {
		name          string
		id            string
		repositoryErr error
		validationErr string
	}{
		{
			name: "success",
			id:   testDeviceID,
		},
		{
			name:          "repository error",
			id:            testDeviceID,
			repositoryErr: device.ErrDeviceNotFound,
		},
		{
			name:          "invalid ID",
			id:            "not-a-uuid",
			validationErr: "device ID must be a valid UUID",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, _ := openMockDatabase(t)
			devices := &deviceRepositoryStub{err: tc.repositoryErr}
			statuses := &statusRepositoryStub{}
			deviceService := service.NewDeviceService(db, devices, statuses)

			ctx := t.Context()
			err := deviceService.Archive(ctx, tc.id)

			if tc.validationErr != "" {
				if err == nil || err.Error() != tc.validationErr {
					t.Fatalf("expected error %q, got: %v", tc.validationErr, err)
				}
				if devices.calls != 0 {
					t.Error("device repository should not be called")
				}
			} else {
				if devices.calls != 1 || devices.method != "Archive" {
					t.Fatal("expected one Archive call")
				}
				if devices.id != tc.id || devices.tx != nil || devices.ctx != ctx {
					t.Error("unexpected repository arguments")
				}

				if tc.repositoryErr != nil {
					if !errors.Is(err, tc.repositoryErr) {
						t.Fatalf("expected repository error, got: %v", err)
					}
				} else if err != nil {
					t.Fatalf("archive device: %v", err)
				}
			}

			if statuses.calls != 0 {
				t.Error("status repository should not be called")
			}
		})
	}
}
