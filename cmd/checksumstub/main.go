package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
)

func main() {
	fail := flag.Bool("fail", false, "simulate checksum failure")
	flag.Parse()

	if err := run(*fail); err != nil {
		fmt.Fprintln(os.Stderr, "checksum stub:", err)
		os.Exit(1)
	}
}

func run(fail bool) error {
	if fail {
		return errors.New("simulated failure")
	}

	const maxInputSize = 64 * 1024
	body, err := io.ReadAll(io.LimitReader(os.Stdin, maxInputSize+1))
	if err != nil {
		return fmt.Errorf("read input: %w", err)
	}
	if len(body) > maxInputSize {
		return errors.New("input exceeds size limit")
	}

	var input struct {
		HardwareVersion string `json:"hardware_version"`
		SoftwareVersion string `json:"software_version"`
		FirmwareVersion string `json:"firmware_version"`
		Status          string `json:"status"`
	}
	if err := json.Unmarshal(body, &input); err != nil {
		return fmt.Errorf("decode input: %w", err)
	}

	canonical, err := json.Marshal(input)
	if err != nil {
		return fmt.Errorf("encode input: %w", err)
	}

	sum := sha256.Sum256(canonical)
	response := struct {
		Checksum string `json:"checksum"`
	}{
		Checksum: hex.EncodeToString(sum[:]),
	}

	if err := json.NewEncoder(os.Stdout).Encode(response); err != nil {
		return fmt.Errorf("write checksum: %w", err)
	}

	return nil
}
