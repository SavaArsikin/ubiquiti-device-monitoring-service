package simulator

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"

	"github.com/SavaArsikin/ubiquiti-device-monitoring-service/internal/deviceprotocol"
)

type checksumGenerator interface {
	Generate(context.Context, deviceprotocol.Diagnostics) (string, error)
}

type CommandChecksumGenerator struct {
	cfg Config
}

func NewCommandChecksumGenerator(cfg Config) (*CommandChecksumGenerator, error) {
	path, err := exec.LookPath(cfg.ChecksumBinary)
	if err != nil {
		return nil, errors.New("checksum executable is unavailable")
	}

	cfg.ChecksumBinary = path
	return &CommandChecksumGenerator{cfg: cfg}, nil
}

func (g *CommandChecksumGenerator) Generate(
	ctx context.Context,
	diagnostics deviceprotocol.Diagnostics,
) (string, error) {
	input := struct {
		HardwareVersion string `json:"hardware_version"`
		SoftwareVersion string `json:"software_version"`
		FirmwareVersion string `json:"firmware_version"`
		Status          string `json:"status"`
	}{
		HardwareVersion: diagnostics.HardwareVersion,
		SoftwareVersion: diagnostics.SoftwareVersion,
		FirmwareVersion: diagnostics.FirmwareVersion,
		Status:          diagnostics.Status,
	}

	body, err := json.Marshal(input)
	if err != nil {
		return "", fmt.Errorf("encode checksum input: %w", err)
	}

	commandCtx, cancel := context.WithTimeout(ctx, g.cfg.ChecksumTimeout)
	defer cancel()

	command := exec.CommandContext(
		commandCtx,
		g.cfg.ChecksumBinary,
		g.cfg.ChecksumArgs...,
	)
	command.Stdin = bytes.NewReader(body)
	command.WaitDelay = g.cfg.ChecksumTimeout

	output := &limitedOutput{remaining: 64 * 1024}
	command.Stdout = output

	if err := command.Run(); err != nil {
		if commandCtx.Err() != nil {
			return "", fmt.Errorf("checksum process interrupted: %w", commandCtx.Err())
		}
		return "", errors.New("checksum process failed")
	}

	var response struct {
		Checksum string `json:"checksum"`
	}
	if err := json.Unmarshal(output.Bytes(), &response); err != nil {
		return "", errors.New("checksum process returned invalid JSON")
	}
	if response.Checksum == "" {
		return "", errors.New("checksum process returned an empty checksum")
	}

	return response.Checksum, nil
}

type limitedOutput struct {
	bytes.Buffer
	remaining int
}

func (w *limitedOutput) Write(p []byte) (int, error) {
	if len(p) > w.remaining {
		return 0, errors.New("checksum output exceeds size limit")
	}
	n, err := w.Buffer.Write(p)
	w.remaining -= n
	return n, err
}
