package main

import (
	"bufio"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/schema"
)

var isScriptRunning bool
var mu sync.Mutex

func doallHandler(w http.ResponseWriter, r *http.Request) {
	mu.Lock()
	if isScriptRunning {
		mu.Unlock()
		http.Error(w, "Script is still running", http.StatusTooEarly)
		return
	}
	isScriptRunning = true
	mu.Unlock()

	defer func() {
		mu.Lock()
		isScriptRunning = false
		mu.Unlock()
	}()

	cmd := exec.Command("/root/antizapret/doall.sh")
	output, err := cmd.CombinedOutput()
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to execute script: %s", err.Error()), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(output)
}

var decoder = schema.NewDecoder()

type ListRequest struct {
	Url          string `schema:"url"`
	File         string `schema:"file"`
	Format       string `schema:"format"`
	Client       string `schema:"client"`        //$client=xxx
	FilterCustom bool   `schema:"filter_custom"` //skip lines with rules from exclude-hosts-custom.txt
	FilterDist   bool   `schema:"filter_dist"`   //skip lines with rules from exclude-hosts-dist.txt
	Allow        bool   `schema:"allow"`         //add @@ at the start of rule
	Raw          bool   `schema:"raw"`           //dont modify rules
	Suffix       bool   `schema:"suffix"`        //add $dnsrewrite,client=xxx to rules
}

type RegexFilter struct {
	cmd      *exec.Cmd
	stdin    io.WriteCloser
	scanner  *bufio.Scanner
	lock     sync.Mutex
	filePath string // sanitized patterns path (in /tmp)
}

// Use a delimiter that is extremely unlikely to occur in real lists.
const delim = "__DELIM__c6b2c4f2-4b6b-4a69-8a9c-9c4ccf58e62a__"

func (rf *RegexFilter) Filter(lines []string) ([]string, error) {
	rf.lock.Lock()
	defer rf.lock.Unlock()

	var result []string

	for _, line := range lines {
		if _, err := fmt.Fprintln(rf.stdin, line); err != nil {
			return result, err
		}
	}
	if _, err := fmt.Fprintln(rf.stdin, delim); err != nil {
		return result, err
	}

	for {
		if !rf.scanner.Scan() {
			return result, rf.scanner.Err()
		}
		text := rf.scanner.Text()
		if text == delim {
			break
		}
		result = append(result, text)
	}
	return result, nil
}

// Close terminates the subprocess cleanly.
func (rf *RegexFilter) Close() error {
	rf.lock.Lock()
	defer rf.lock.Unlock()

	if rf.stdin != nil {
		// Ensure at least one line is processed by grep to avoid exit code 1
		_, _ = rf.Filter([]string{"example.com"})
		_ = rf.stdin.Close()
		rf.stdin = nil
	}
	if rf.cmd != nil {
		err := rf.cmd.Wait()
		rf.cmd = nil
		return err
	}
	return nil
}

// sanitizePatternFile:
// - removes UTF-8 BOM
// - normalizes CRLF/CR -> LF
// - trims whitespace
// - drops empty lines
// - drops full-line comments starting with '#' or '!'
//
// NOTE: We intentionally do NOT remove inline comments, as '#' may be meaningful inside regex.
func sanitizePatternFile(srcPath string) (string, error) {
	data, err := os.ReadFile(srcPath)
	if err != nil {
		return "", err
	}

	// Remove UTF-8 BOM if present
	if len(data) >= 3 && data[0] == 0xEF && data[1] == 0xBB && data[2] == 0xBF {
		data = data[3:]
	}

	// Normalize line endings
	s := string(data)
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")

	var outLines []string
	sc := bufio.NewScanner(strings.NewReader(s))
	// pattern files are usually short; but set a safe limit anyway
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "#") || strings.HasPrefix(line, "!") {
			continue
		}
		outLines = append(outLines, line)
	}
	if err := sc.Err(); err != nil {
		return "", err
	}

	// stable temp file name based on content hash (avoid rewriting same content)
	sum := sha256.Sum256([]byte(strings.Join(outLines, "\n")))
	name := "exclude-sanitized-" + hex.EncodeToString(sum[:8]) + ".txt"
	dstPath := filepath.Join(os.TempDir(), name)

	if _, err := os.Stat(dstPath); err == nil {
		return dstPath, nil
	}

	content := strings.Join(outLines, "\n") + "\n"
	if err := os.WriteFile(dstPath, []byte(content), 0644); err != nil {
		return "", err
	}
	return dstPath, nil
}

// validateGrepERE validates that the pattern file is accepted by `grep -E -f`.
// grep exit code 2 indicates a regex error.
func validateGrepERE(patternFile string) error {
	var stderr strings.Builder
	// We don't care about matching; we care that grep accepts the patterns.
	cmd := exec.Command("grep", "-E", "-f", patternFile, "-q", "a")
	cmd.Stdin = strings.NewReader("a\n")
	cmd.Stdout = io.Discard
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err == nil {
		return nil
	}

	if ee, ok := err.(*exec.ExitError); ok && ee.ExitCode() == 2 {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = "regex error"
		}
		return fmt.Errorf("invalid grep -E regex in %s: %s", patternFile, msg)
	}
	// Other errors (e.g. grep not found) are also important
	msg := strings.TrimSpace(stderr.String())
	if msg != "" {
		return fmt.Errorf("grep validation failed for %s: %v (%s)", patternFile, err, msg)
	}
	return fmt.Errorf("grep validation failed for %s: %v", patternFile, err)
}

func NewRegexFilter(file string) (*RegexFilter, error) {
	sanitized, err := sanitizePatternFile(file)
	if err != nil {
		return nil, fmt.Errorf("sanitize patterns %s: %w", file, err)
	}
	if err := validateGrepERE(sanitized); err != nil {
		return nil, err
	}

	cmd := exec.Command(
		"grep",
		"--line-buffered",
		"-v",
		"-E",
		"-f",
		sanitized,
	)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}

	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return nil, err
	}

	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024) // allow long lines

	return &RegexFilter{
		cmd:      cmd,
		stdin:    stdin,
		scanner:  scanner,
		lock:     sync.Mutex{},
		filePath: sanitized,
	}, nil
}

var excludeMatcherDist *RegexFilter
var excludeMatcherCustom *RegexFilter

var DefaultClient string

func adaptList(w http.ResponseWriter, r *http.Request) {
	req := ListRequest{
		Client:       DefaultClient,
		FilterCustom: true,
		FilterDist:   false,
		Allow:        true, // default (adds @@)
		Suffix:       true,
		Raw:          false,
	}

	if err := decoder.Decode(&req, r.URL.Query()); err != nil {
		http.Error(w, fmt.Sprintf("Invalid request body: %v", err), http.StatusBadRequest)
		return
	}

	var reader io.ReadCloser
	if req.Url != "" {
		reqRemote, err := http.NewRequest("GET", req.Url, nil)
		if err != nil {
			http.Error(w, fmt.Sprintf("Failed to create request: %v", err), http.StatusInternalServerError)
			return
		}

		// Forward all headers from the original request
		for name, values := range r.Header {
			for _, value := range values {
				reqRemote.Header.Add(name, value)
			}
		}

		client := &http.Client{}
		resp, err := client.Do(reqRemote)
		if err != nil {
			http.Error(w, fmt.Sprintf("Failed to download list: %v", err), http.StatusInternalServerError)
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			http.Error(w, fmt.Sprintf("Remote server returned %d", resp.StatusCode), http.StatusBadGateway)
			return
		}

		if resp.Header.Get("Content-Encoding") == "gzip" {
			gz, err := gzip.NewReader(resp.Body)
			if err != nil {
				http.Error(w, fmt.Sprintf("Cant uncompress response: %v", err), http.StatusInternalServerError)
				return
			}
			defer gz.Close()
			reader = gz
		} else {
			reader = resp.Body
		}

		if resp.Header.Get("Content-Type") == "application/json" && req.Format == "" {
			req.Format = "json"
		}
	} else if req.File != "" {
		file, err := os.Open(req.File)
		if err != nil {
			http.Error(w, fmt.Sprintf("Failed to open local file: %v", err), http.StatusInternalServerError)
			return
		}
		reader = file
	} else {
		http.Error(w, "Url or File required", http.StatusBadRequest)
		return
	}
	defer reader.Close()

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)

	var buffer []string

	processBuffer := func() {
		if len(buffer) == 0 {
			return
		}
		filtered := buffer
		buffer = nil

		if req.FilterDist {
			var err error
			filtered, err = excludeMatcherDist.Filter(filtered)
			if err != nil {
				log.Printf("[WARN] dist exclude filter error: %v", err)
			}
		}
		if req.FilterCustom {
			var err error
			filtered, err = excludeMatcherCustom.Filter(filtered)
			if err != nil {
				log.Printf("[WARN] custom exclude filter error: %v", err)
			}
		}

		for _, line := range filtered {
			out := strings.TrimSpace(line)

			if req.Raw || out == "" || strings.HasPrefix(out, "!") || strings.HasPrefix(out, "#") {
				// pass through as-is (or blank)
			} else {
				if !strings.HasPrefix(line, "/") {
					out = "||" + out + "^"
				}
				if req.Suffix {
					out = fmt.Sprintf("%s$dnsrewrite,client=%s", out, req.Client)
				}
				if req.Allow {
					out = "@@" + out
				}
			}

			fmt.Fprintln(w, out)
		}
	}

	processLine := func(line string) {
		buffer = append(buffer, line)
		if len(buffer) > 1000 {
			processBuffer()
		}
	}

	if req.Format == "" {
		req.Format = "list"
	}

	switch strings.ToLower(req.Format) {
	case "list":
		scanner := bufio.NewScanner(reader)
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		for scanner.Scan() {
			processLine(scanner.Text())
		}
		if err := scanner.Err(); err != nil {
			fmt.Fprintf(w, "# Error reading list: %v\n", err)
		}
	case "json":
		dec := json.NewDecoder(reader)

		t, err := dec.Token()
		if err != nil {
			http.Error(w, fmt.Sprintf("Invalid JSON: %v", err), http.StatusBadRequest)
			return
		}
		if d, ok := t.(json.Delim); !ok || d != '[' {
			http.Error(w, "Expected JSON array", http.StatusBadRequest)
			return
		}

		for dec.More() {
			var item string
			if err := dec.Decode(&item); err != nil {
				http.Error(w, fmt.Sprintf("# Error decoding JSON item: %v\n", err), http.StatusBadRequest)
				break
			}
			processLine(item)
		}
		_, _ = dec.Token()
	default:
		http.Error(w, "Unsupported format (use 'json' or 'list')", http.StatusBadRequest)
		return
	}

	processBuffer()
	flusher.Flush()
}

func updateRegexFilter() error {
	var err error

	if excludeMatcherDist != nil {
		if e := excludeMatcherDist.Close(); e != nil {
			return e
		}
	}
	excludeMatcherDist, err = NewRegexFilter("/root/antizapret/config/exclude-hosts-dist.txt")
	if err != nil {
		return err
	}

	if excludeMatcherCustom != nil {
		if e := excludeMatcherCustom.Close(); e != nil {
			return e
		}
	}
	excludeMatcherCustom, err = NewRegexFilter("/root/antizapret/config/custom/exclude-hosts-custom.txt")
	return err
}

func update(w http.ResponseWriter, r *http.Request) {
	err := updateRegexFilter()
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to update exclude lists: %v", err), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

// responseWriterWrapper captures the status code and bytes written.
type responseWriterWrapper struct {
	http.ResponseWriter
	statusCode int
	bytesSent  int
}

func (rw *responseWriterWrapper) WriteHeader(code int) {
	if rw.statusCode != 0 {
		return
	}
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

func (rw *responseWriterWrapper) Write(b []byte) (int, error) {
	if rw.statusCode == 0 {
		rw.WriteHeader(http.StatusOK)
	}
	n, err := rw.ResponseWriter.Write(b)
	rw.bytesSent += n
	return n, err
}

func (rw *responseWriterWrapper) Flush() {
	if flusher, ok := rw.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		ip := r.RemoteAddr
		if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
			ip = forwarded
		}

		wrapped := &responseWriterWrapper{ResponseWriter: w}

		log.Printf("[REQ] %s %s?%s from %s", r.Method, r.URL.Path, r.URL.RawQuery, ip)
		next.ServeHTTP(wrapped, r)
		duration := time.Since(start)
		log.Printf("[RES] %s %s?%s -> %d (%d bytes, %v)", r.Method, r.URL.Path, r.URL.RawQuery, wrapped.statusCode, wrapped.bytesSent, duration)
	})
}

func main() {
	DefaultClient = os.Getenv("CLIENT")
	runtime.GOMAXPROCS(runtime.NumCPU())

	err := updateRegexFilter()
	if err != nil {
		log.Fatalf("Failed to initialize regex filters: %v", err)
	}
	defer func() {
		if excludeMatcherDist != nil {
			_ = excludeMatcherDist.Close()
		}
		if excludeMatcherCustom != nil {
			_ = excludeMatcherCustom.Close()
		}
	}()

	r := http.NewServeMux()
	r.HandleFunc(`/list/`, adaptList)
	r.HandleFunc(`/doall/`, doallHandler)
	r.HandleFunc(`/update/`, update)

	fmt.Println("Starting server on http://localhost:80")
	log.Fatal(http.ListenAndServe(":80", loggingMiddleware(r)))
}
