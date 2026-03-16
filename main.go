/*
CHUNK HUNTER (GUI version)

	Scans text files for a set of predefined chunks.
	Runs a local web server and opens the browser for a graphical interface.
	More information: https://github.com/TomDeneire/chunkhunter
*/
package main

import (
	"archive/zip"
	"bufio"
	"bytes"
	_ "embed"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"unicode"

	"golang.org/x/net/html/charset"
	"golang.org/x/text/encoding/charmap"
)

//go:embed static/index.html
var indexHTML []byte

// chunks holds the loaded chunk database
var chunks map[string]int

// exeDir returns the directory where chunks.txt should be found.
// Uses the executable's directory (for deployed .exe), but falls back
// to the working directory when run via "go run" (temp build cache).
func exeDir() string {
	exe, err := os.Executable()
	if err != nil {
		return "."
	}
	dir := filepath.Dir(exe)
	// "go run" places the binary in a temp/cache directory — fall back to cwd
	if strings.Contains(dir, "go-build") || strings.Contains(dir, "go_build") {
		return "."
	}
	return dir
}

// --- Chunk loading ---

func cleanChunk(s string) string {
	clean := strings.TrimSpace(s)
	lower := strings.ToLower(clean)
	return " " + lower // leading space prevents partial matches (e.g. "was" in "always")
}

func loadChunks() map[string]int {
	dbMap := make(map[string]int)
	path := filepath.Join(exeDir(), "chunks.txt")
	f, err := os.Open(path)
	if err != nil {
		log.Fatalf("Unable to open chunks database at %s: %v", path, err)
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		chunk := cleanChunk(scanner.Text())
		if strings.TrimSpace(chunk) != "" {
			dbMap[chunk] = 1
		}
	}
	return dbMap
}

// --- Document text extraction ---

func extractText(filename string, data []byte) (string, error) {
	ext := strings.ToLower(filepath.Ext(filename))
	switch ext {
	case ".txt":
		return decodeText(data), nil
	case ".docx":
		return extractDOCX(data)
	case ".odt":
		return extractODT(data)
	case ".doc":
		return extractDOC(data)
	default:
		return "", fmt.Errorf("unsupported file format: %s", ext)
	}
}

// decodeText tries to interpret bytes as UTF-8, falling back to Windows-1252.
func decodeText(data []byte) string {
	_, name, _ := charset.DetermineEncoding(data, "text/plain")
	if name == "windows-1252" || name == "iso-8859-1" {
		decoded, err := charmap.Windows1252.NewDecoder().Bytes(data)
		if err == nil {
			return string(decoded)
		}
	}
	return string(data)
}

// extractDOCX reads text from a .docx file (ZIP containing word/document.xml).
func extractDOCX(data []byte) (string, error) {
	r, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return "", fmt.Errorf("cannot read docx: %v", err)
	}
	for _, f := range r.File {
		if f.Name == "word/document.xml" {
			rc, err := f.Open()
			if err != nil {
				return "", err
			}
			defer rc.Close()
			xmlData, err := io.ReadAll(rc)
			if err != nil {
				return "", err
			}
			return extractXMLText(string(xmlData), "w:p", "w:t"), nil
		}
	}
	return "", fmt.Errorf("word/document.xml not found in docx")
}

// extractODT reads text from an .odt file (ZIP containing content.xml).
func extractODT(data []byte) (string, error) {
	r, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return "", fmt.Errorf("cannot read odt: %v", err)
	}
	for _, f := range r.File {
		if f.Name == "content.xml" {
			rc, err := f.Open()
			if err != nil {
				return "", err
			}
			defer rc.Close()
			xmlData, err := io.ReadAll(rc)
			if err != nil {
				return "", err
			}
			return extractXMLText(string(xmlData), "text:p", ""), nil
		}
	}
	return "", fmt.Errorf("content.xml not found in odt")
}

// extractXMLText is a simple XML text extractor. It collects character data
// and inserts newlines after closing paragraph tags.
// paragraphTag is the local name of paragraph elements (e.g. "w:p" or "text:p").
// textTag optionally filters to only collect text inside that tag (e.g. "w:t"); empty means collect all.
func extractXMLText(xmlStr string, paragraphTag string, textTag string) string {
	var result strings.Builder
	s := xmlStr

	// Simple state-machine XML parser (avoids encoding/xml issues with namespaces)
	for len(s) > 0 {
		idx := strings.Index(s, "<")
		if idx < 0 {
			// Remaining text (outside tags)
			if textTag == "" {
				result.WriteString(s)
			}
			break
		}

		// Text before the tag
		textBefore := s[:idx]
		s = s[idx:]

		if textTag == "" && len(textBefore) > 0 {
			result.WriteString(textBefore)
		}

		// Find end of tag
		end := strings.Index(s, ">")
		if end < 0 {
			break
		}
		tag := s[1:end]
		s = s[end+1:]

		// Check for closing paragraph tag → newline
		if strings.HasPrefix(tag, "/"+paragraphTag) {
			result.WriteString("\n")
			continue
		}

		// For textTag mode: collect text content inside the text element
		if textTag != "" && strings.HasPrefix(tag, textTag) && !strings.HasSuffix(tag, "/") {
			// Find the closing tag
			closeTag := "</" + textTag + ">"
			closeIdx := strings.Index(s, closeTag)
			if closeIdx >= 0 {
				result.WriteString(s[:closeIdx])
				s = s[closeIdx+len(closeTag):]
			}
		}
	}

	return result.String()
}

// extractDOC attempts best-effort text extraction from legacy .doc binary files.
// The Word Binary Format is complex; this extracts readable text runs.
func extractDOC(data []byte) (string, error) {
	if len(data) < 512 {
		return "", fmt.Errorf("file too small to be a valid .doc")
	}

	// Try UTF-16LE extraction first (common in Word 97-2003)
	text := extractUTF16LE(data)
	if len(text) > 50 {
		return text, nil
	}

	// Fallback: extract ASCII runs
	return extractASCIIRuns(data), nil
}

// extractUTF16LE scans binary data for UTF-16LE encoded text.
func extractUTF16LE(data []byte) string {
	var result strings.Builder
	var current strings.Builder

	for i := 0; i+1 < len(data); i += 2 {
		lo := data[i]
		hi := data[i+1]
		if hi == 0 && (lo >= 0x20 && lo < 0x7F || lo == '\n' || lo == '\r' || lo == '\t') {
			current.WriteByte(lo)
		} else {
			if current.Len() > 10 {
				result.WriteString(current.String())
				result.WriteByte('\n')
			}
			current.Reset()
		}
	}
	if current.Len() > 10 {
		result.WriteString(current.String())
	}
	return result.String()
}

// extractASCIIRuns extracts long runs of printable ASCII from binary data.
func extractASCIIRuns(data []byte) string {
	var result strings.Builder
	var current strings.Builder

	for _, b := range data {
		if b >= 0x20 && b < 0x7F || b == '\n' || b == '\r' || b == '\t' {
			current.WriteByte(b)
		} else {
			s := current.String()
			if len(s) > 20 && hasReadableWords(s) {
				result.WriteString(s)
				result.WriteByte('\n')
			}
			current.Reset()
		}
	}
	s := current.String()
	if len(s) > 20 && hasReadableWords(s) {
		result.WriteString(s)
	}
	return result.String()
}

// hasReadableWords checks if a string contains actual words (not just binary junk).
func hasReadableWords(s string) bool {
	words := strings.Fields(s)
	if len(words) < 3 {
		return false
	}
	letters := 0
	for _, r := range s {
		if unicode.IsLetter(r) {
			letters++
		}
	}
	return float64(letters)/float64(len(s)) > 0.5
}

// --- Chunk scanning ---

type FileResult struct {
	Filename   string  `json:"filename"`
	HTML       string  `json:"html"`
	Words      int     `json:"words"`
	ChunkWords int     `json:"chunkWords"`
	Percent    float64 `json:"percent"`
	CSV        string  `json:"csv"`
}

type ScanResponse struct {
	Results     []FileResult `json:"results"`
	TotalChunks int          `json:"totalChunks"`
	Error       string       `json:"error,omitempty"`
}

// matchPos records where a chunk was found in the text.
type matchPos struct {
	start int
	end   int
}

func scanText(inputText string) FileResult {
	words := len(strings.Fields(inputText))
	inputWithSpace := " " + inputText // leading space for chunk boundary matching
	inputCheck := strings.ToLower(inputWithSpace)
	chunkTotal := 0

	records := [][]string{{"chunk", "length", "frequency"}}

	// Collect all match positions for highlighting
	var matches []matchPos

	for chunk := range chunks {
		if !strings.Contains(inputCheck, chunk) {
			continue
		}
		chunkFreq := strings.Count(inputCheck, chunk)

		// Find all positions of this chunk in the text
		searchFrom := 0
		for {
			idx := strings.Index(inputCheck[searchFrom:], chunk)
			if idx < 0 {
				break
			}
			absStart := searchFrom + idx
			absEnd := absStart + len(chunk)
			matches = append(matches, matchPos{start: absStart, end: absEnd})
			searchFrom = absStart + 1
		}

		// CSV record
		chunkString := strings.TrimSpace(chunk)
		record := []string{chunkString, strconv.Itoa(len(chunk)), strconv.Itoa(chunkFreq)}
		records = append(records, record)

		// Count
		chunkTotal += len(strings.Split(chunkString, " ")) * chunkFreq
	}

	// Build highlighted HTML using match positions
	// Create a coverage array marking which bytes are highlighted
	highlighted := make([]bool, len(inputWithSpace))
	for _, m := range matches {
		for i := m.start; i < m.end && i < len(highlighted); i++ {
			highlighted[i] = true
		}
	}

	// Build HTML output
	var htmlBuf strings.Builder
	inSpan := false
	for i, b := range []byte(inputWithSpace) {
		if highlighted[i] && !inSpan {
			htmlBuf.WriteString(`<span style="background-color: #FFFF00">`)
			inSpan = true
		} else if !highlighted[i] && inSpan {
			htmlBuf.WriteString(`</span>`)
			inSpan = false
		}
		htmlBuf.WriteByte(b)
	}
	if inSpan {
		htmlBuf.WriteString(`</span>`)
	}

	// Generate CSV string
	var csvBuf bytes.Buffer
	w := csv.NewWriter(&csvBuf)
	w.WriteAll(records)

	percent := 0.0
	if words > 0 {
		percent = float64(chunkTotal) / float64(words) * 100
	}

	htmlResult := htmlBuf.String()
	if len(htmlResult) > 0 {
		// Remove the leading space we added
		if htmlResult[0] == ' ' {
			htmlResult = htmlResult[1:]
		} else if strings.HasPrefix(htmlResult, `<span style="background-color: #FFFF00"> `) {
			// Leading space is inside a span — remove just the space
			htmlResult = `<span style="background-color: #FFFF00">` + htmlResult[len(`<span style="background-color: #FFFF00"> `):]
		}
	}

	return FileResult{
		HTML:       htmlResult,
		Words:      words,
		ChunkWords: chunkTotal,
		Percent:    percent,
		CSV:        csvBuf.String(),
	}
}

// --- HTTP handlers ---

func handleIndex(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(indexHTML)
}

func handleScan(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}

	// Parse multipart form (max 50 MB)
	err := r.ParseMultipartForm(50 << 20)
	if err != nil {
		writeJSON(w, ScanResponse{Error: "Failed to parse upload: " + err.Error()})
		return
	}

	fileHeaders := r.MultipartForm.File["files"]
	if len(fileHeaders) == 0 {
		writeJSON(w, ScanResponse{Error: "No files uploaded"})
		return
	}

	var results []FileResult

	for _, fh := range fileHeaders {
		f, err := fh.Open()
		if err != nil {
			results = append(results, FileResult{Filename: fh.Filename})
			continue
		}
		data, err := io.ReadAll(f)
		f.Close()
		if err != nil {
			results = append(results, FileResult{Filename: fh.Filename})
			continue
		}

		text, err := extractText(fh.Filename, data)
		if err != nil {
			results = append(results, FileResult{
				Filename: fh.Filename,
				HTML:     "Error: " + err.Error(),
			})
			continue
		}

		result := scanText(text)
		result.Filename = fh.Filename
		results = append(results, result)
	}

	writeJSON(w, ScanResponse{
		Results:     results,
		TotalChunks: len(chunks),
	})
}

func handleScanText(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Text string `json:"text"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, ScanResponse{Error: "Invalid request: " + err.Error()})
		return
	}
	if strings.TrimSpace(req.Text) == "" {
		writeJSON(w, ScanResponse{Error: "No text provided"})
		return
	}

	result := scanText(req.Text)
	result.Filename = "Pasted text"

	writeJSON(w, ScanResponse{
		Results:     []FileResult{result},
		TotalChunks: len(chunks),
	})
}

func writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

// --- Browser launch ---

func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	cmd.Start()
}

// --- Main ---

func main() {
	log.Println("Loading chunks database...")
	chunks = loadChunks()
	log.Printf("Loaded %d unique chunks", len(chunks))

	http.HandleFunc("/", handleIndex)
	http.HandleFunc("/api/scan", handleScan)
	http.HandleFunc("/api/scan-text", handleScanText)

	// Find an available port
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	url := fmt.Sprintf("http://127.0.0.1:%d", port)

	log.Printf("Starting Chunk Hunter at %s", url)
	openBrowser(url)

	log.Fatal(http.Serve(listener, nil))
}
