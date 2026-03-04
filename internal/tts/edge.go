package tts

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"time"
)

// EdgeProvider implements TTS via Microsoft Edge TTS (free, no API key).
// Matching TS edgeTTS() in src/tts/tts-core.ts.
// Requires the `edge-tts` CLI tool to be installed:
//
//	pip install edge-tts
type EdgeProvider struct {
	voice     string // default "en-US-MichelleNeural"
	rate      string // speech rate, e.g. "+0%"
	timeoutMs int
}

// EdgeConfig configures the Edge TTS provider.
type EdgeConfig struct {
	Voice     string
	Rate      string
	TimeoutMs int
}

// NewEdgeProvider creates an Edge TTS provider.
func NewEdgeProvider(cfg EdgeConfig) *EdgeProvider {
	p := &EdgeProvider{
		voice:     cfg.Voice,
		rate:      cfg.Rate,
		timeoutMs: cfg.TimeoutMs,
	}
	if p.voice == "" {
		p.voice = "en-US-MichelleNeural"
	}
	if p.timeoutMs <= 0 {
		p.timeoutMs = 30000
	}
	return p
}

func (p *EdgeProvider) Name() string { return "edge" }

// Synthesize runs the edge-tts CLI to generate audio.
// Edge TTS natively outputs MP3. When opts.Format is "opus", the MP3 is
// converted to Opus via ffmpeg (must be installed on the host).
func (p *EdgeProvider) Synthesize(ctx context.Context, text string, opts Options) (*SynthResult, error) {
	outFile, err := os.CreateTemp("", "tts-*.mp3")
	if err != nil {
		return nil, fmt.Errorf("edge-tts: create temp file: %w", err)
	}
	outPath := outFile.Name()
	outFile.Close()
	defer os.Remove(outPath)

	args := []string{
		"--voice", p.voice,
		"--text", text,
		"--write-media", outPath,
	}
	if p.rate != "" {
		args = append(args, "--rate", p.rate)
	}

	timeout := time.Duration(p.timeoutMs) * time.Millisecond
	cmdCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(cmdCtx, "edge-tts", args...)
	if output, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("edge-tts failed: %w (output: %s)", err, string(output))
	}

	audio, err := os.ReadFile(outPath)
	if err != nil {
		return nil, fmt.Errorf("read edge-tts output: %w", err)
	}

	// Convert MP3→Opus via ffmpeg when opus format is requested (e.g. voicebox channel).
	// Use fresh timeout for ffmpeg — don't reuse cmdCtx which may have little time left.
	if opts.Format == "opus" {
		ffCtx, ffCancel := context.WithTimeout(ctx, 15*time.Second)
		defer ffCancel()
		audio, err = convertMP3ToOpus(ffCtx, audio)
		if err != nil {
			return nil, fmt.Errorf("edge-tts opus conversion: %w", err)
		}
		return &SynthResult{
			Audio:     audio,
			Extension: "opus",
			MimeType:  "audio/ogg",
		}, nil
	}

	return &SynthResult{
		Audio:     audio,
		Extension: "mp3",
		MimeType:  "audio/mpeg",
	}, nil
}

// convertMP3ToOpus pipes MP3 bytes through ffmpeg to produce OGG/Opus output.
func convertMP3ToOpus(ctx context.Context, mp3 []byte) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "ffmpeg",
		"-i", "pipe:0", // read MP3 from stdin
		"-c:a", "libopus",
		"-b:a", "48k",
		"-ar", "24000",
		"-ac", "1",
		"-f", "ogg",
		"pipe:1", // write Opus to stdout
	)
	cmd.Stdin = bytes.NewReader(mp3)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("ffmpeg mp3→opus: %w (stderr: %s)", err, stderr.String())
	}
	return out, nil
}
