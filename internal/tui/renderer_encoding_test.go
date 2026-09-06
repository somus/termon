package tui

import (
	"bytes"
	"encoding/json"
	"image/color"
	"os"
	"runtime"
	"testing"

	"github.com/charmbracelet/colorprofile"
	uv "github.com/charmbracelet/ultraviolet"
)

func TestRendererSkipsUnchangedColoredInterior(t *testing.T) {
	var output bytes.Buffer
	renderer := uv.NewTerminalRenderer(&output, []string{"TERM=xterm-256color"})
	renderer.SetColorProfile(colorprofile.TrueColor)
	renderer.SetFullscreen(true)
	renderer.Resize(40, 1)
	buffer := uv.NewRenderBuffer(40, 1)
	for x := range 40 {
		cell := uv.Cell{Content: " ", Width: 1, Style: uv.Style{Bg: color.RGBA{R: uint8(x % 2 * 255), A: 255}}}
		buffer.SetCell(x, 0, &cell)
	}
	renderer.Render(buffer)
	if err := renderer.Flush(); err != nil {
		t.Fatal(err)
	}
	output.Reset()
	cell := uv.Cell{Content: "X", Width: 1}
	buffer.SetCell(0, 0, &cell)
	buffer.SetCell(39, 0, &cell)
	renderer.Render(buffer)
	if err := renderer.Flush(); err != nil {
		t.Fatal(err)
	}
	if output.Len() > 80 {
		t.Fatalf("re-emitted unchanged colored cells: %d bytes", output.Len())
	}
}

type rendererTraceFrame struct {
	Reset   bool   `json:"reset"`
	Width   int    `json:"width"`
	Height  int    `json:"height"`
	Delta   string `json:"delta"`
	Full    string `json:"full"`
	CursorX int    `json:"cursor_x"`
	CursorY int    `json:"cursor_y"`
}

// TestExportLobbyRendererTrace exports deterministic incremental and full
// repaints for independent terminal-emulator comparison. It is opt-in because
// the normal Go suite does not require Python or a terminal emulator.
func TestExportLobbyRendererTrace(t *testing.T) {
	path := os.Getenv("TERMON_RENDER_TRACE")
	if path == "" {
		t.Skip("set TERMON_RENDER_TRACE to a new JSON file")
	}
	var traces []rendererTraceFrame
	for _, profile := range []colorprofile.Profile{colorprofile.ANSI256, colorprofile.TrueColor} {
		var output bytes.Buffer
		renderer := uv.NewTerminalRenderer(&output, []string{"TERM=xterm-256color"})
		renderer.SetColorProfile(profile)
		renderer.SetScrollOptim(runtime.GOOS != "windows")
		renderer.SetFullscreen(true)
		renderer.Erase()
		m := memoLobbyModel()
		m.width, m.height = 120, 40
		m.snap.You.X, m.snap.You.Y = 8, 12
		m.lobbyCameraOn = false
		for step := range 80 {
			switch {
			case step < 25:
				m.snap.You.X = 8 + step
			case step < 50:
				m.snap.You.X = 32 - (step - 25)
			case step < 60:
				m.snap.You.Y = 12 - (step - 50)
			case step < 70:
				m.snap.You.Y = 3 + step - 60
			case step == 70:
				m.width, m.height = 80, 24
			case step == 75:
				m.width, m.height = 120, 40
			}
			if step == 35 {
				m.snap.You.Handle = "alpha界"
			}
			if step == 55 {
				m.snap.You.Handle = "a\u0301lpha"
			}
			if step == 0 || step == 70 || step == 75 {
				renderer.Resize(m.width, m.height)
			}
			m.syncLobbyCamera(m.snap.You, step == 70 || step == 75)
			frame := m.buildFrame()
			buffer := uv.NewScreenBuffer(m.width, m.height)
			uv.NewStyledString(frame).Draw(buffer, buffer.Bounds())
			renderer.Render(buffer.RenderBuffer)
			if err := renderer.Flush(); err != nil {
				t.Fatal(err)
			}
			var full bytes.Buffer
			reference := uv.NewTerminalRenderer(&full, []string{"TERM=xterm-256color"})
			reference.SetColorProfile(profile)
			reference.SetScrollOptim(runtime.GOOS != "windows")
			reference.SetFullscreen(true)
			reference.Erase()
			reference.Resize(m.width, m.height)
			buffer = uv.NewScreenBuffer(m.width, m.height)
			uv.NewStyledString(frame).Draw(buffer, buffer.Bounds())
			reference.Render(buffer.RenderBuffer)
			if err := reference.Flush(); err != nil {
				t.Fatal(err)
			}
			cursorX, cursorY := renderer.Position()
			traces = append(traces, rendererTraceFrame{step == 0, m.width, m.height, output.String(), full.String(), cursorX, cursorY})
			output.Reset()
		}
	}
	data, err := json.Marshal(traces)
	if err != nil {
		t.Fatal(err)
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.Write(data); err != nil {
		t.Error(err)
	}
	if err := file.Close(); err != nil {
		t.Error(err)
	}
}
