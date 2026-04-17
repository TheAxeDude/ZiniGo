package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/icza/gox/stringsx"
	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
	"io/ioutil"
	"log"
	"math/rand"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const zinioBase = "https://www.zinio.com"

// LoginResponse is the response from POST /api/x7b9q-sync
type LoginResponse struct {
	Status bool      `json:"status"`
	Data   LoginData `json:"data"`
}

type LoginData struct {
	User LoginUser `json:"user"`
}

type LoginUser struct {
	UserIDString string `json:"user_id_string"`
	Email        string `json:"email"`
}

// LibraryResponse is the paginated response from /api/newsstand/newsstands/101/users/{id}/library-issues
type LibraryResponse struct {
	Metadata LibraryMeta    `json:"metadata"`
	Data     []LibraryIssue `json:"data"`
}

type LibraryMeta struct {
	Results int `json:"results"`
	Offset  int `json:"offset"`
	Limit   int `json:"limit"`
}

type LibraryIssue struct {
	Id          int        `json:"id"`
	Name        string     `json:"name"`
	Publication LibraryPub `json:"publication"`
}

type LibraryPub struct {
	Id            int    `json:"id"`
	Name          string `json:"name"`
	LegacyContent int    `json:"legacy_content"`
}

// ReaderContent is the response from /api/reader/content
type ReaderContent struct {
	Data ReaderData `json:"data"`
}

type ReaderData struct {
	Issue ReaderIssue  `json:"issue"`
	Pages []ReaderPage `json:"pages"`
}

type ReaderIssue struct {
	LegacyHash  string `json:"legacy_hash"`
	Hash        string `json:"hash"`
	Publication struct {
		LegacyContent int `json:"legacy_content"`
	} `json:"publication"`
}

type ReaderPage struct {
	Index string `json:"index"`
	Src   string `json:"src"`
}

// ZinioClient holds HTTP session state and credentials for re-authentication.
type ZinioClient struct {
	http        *http.Client
	username    string
	password    string
	fingerprint string
	newsstandID int
	userID      string
}

func newZinioClient(username, password, fingerprint string, newsstandID int) *ZinioClient {
	jar, err := cookiejar.New(nil)
	if err != nil {
		log.Fatalf("failed to create cookie jar: %v", err)
	}
	return &ZinioClient{
		http: &http.Client{
			Jar:     jar,
			Timeout: 30 * time.Second,
		},
		username:    username,
		password:    password,
		fingerprint: fingerprint,
		newsstandID: newsstandID,
	}
}

func (z *ZinioClient) relogin() bool {
	delays := []time.Duration{5 * time.Second, 15 * time.Second, 60 * time.Second}
	for attempt, delay := range delays {
		fmt.Printf("Re-authenticating (attempt %d/%d)...\n", attempt+1, len(delays))
		resp, err := login(z.http, z.username, z.password, z.fingerprint, z.newsstandID)
		if err == nil && resp.Status && resp.Data.User.UserIDString != "" {
			z.userID = resp.Data.User.UserIDString
			fmt.Println("Re-authenticated as:", resp.Data.User.Email)
			return true
		}
		fmt.Printf("Re-login failed: %v — waiting %s before retry\n", err, delay)
		time.Sleep(delay)
	}
	fmt.Println("Re-authentication failed after all attempts")
	return false
}

func main() {
	usernamePtr := flag.String("u", "", "Zinio Username")
	passwordPtr := flag.String("p", "", "Zinio Password")
	deviceFingerprintPtr := flag.String("fingerprint", "abcd123", "Device fingerprint")
	newsstandIDPtr := flag.Int("ns", 101, "Newsstand ID")

	flag.Parse()

	mydir, err := os.Getwd()
	if err != nil {
		fmt.Println(err)
	}

	if fileExists(mydir + "/config.json") {
		fmt.Println("Config file loaded")
		byteValue, readErr := ioutil.ReadFile("config.json")
		if readErr != nil {
			log.Fatalf("failed to read config.json: %v", readErr)
		}

		if u := gjson.GetBytes(byteValue, "username"); u.Exists() {
			*usernamePtr = u.String()
			fmt.Println("Username taken from config file")
		}
		if p := gjson.GetBytes(byteValue, "password"); p.Exists() {
			*passwordPtr = p.String()
			fmt.Println("Password taken from config file")
		}
		if fp := gjson.GetBytes(byteValue, "fingerprint"); fp.Exists() {
			*deviceFingerprintPtr = fp.String()
			fmt.Println("Fingerprint taken from config file")
		} else {
			fmt.Println("No fingerprint in config, generating one")
			newJson, _ := sjson.Set(string(byteValue), "fingerprint", randSeq(15))
			if writeErr := ioutil.WriteFile("config.json", []byte(newJson), 0644); writeErr != nil {
				log.Fatalf("unable to write file: %v", writeErr)
			}
			*deviceFingerprintPtr = gjson.Get(newJson, "fingerprint").String()
		}
	}

	if *usernamePtr == "" || *passwordPtr == "" {
		log.Fatal("Username and password are required. Use -u and -p flags or config.json")
	}

	zc := newZinioClient(*usernamePtr, *passwordPtr, *deviceFingerprintPtr, *newsstandIDPtr)

	loginResp, loginErr := login(zc.http, zc.username, zc.password, zc.fingerprint, zc.newsstandID)
	if loginErr != nil {
		log.Fatalf("Login failed: %v", loginErr)
	}
	if !loginResp.Status || loginResp.Data.User.UserIDString == "" {
		log.Fatal("Login failed: server returned status=false")
	}
	zc.userID = loginResp.Data.User.UserIDString
	fmt.Println("Logged in as:", loginResp.Data.User.Email, "| UserID:", zc.userID)

	issueDirectory := filepath.Join(mydir, "issue")
	if _, statErr := os.Stat(issueDirectory); os.IsNotExist(statErr) {
		if mkdirErr := os.Mkdir(issueDirectory, 0700); mkdirErr != nil {
			log.Fatalf("unable to create issue directory %q: %v", issueDirectory, mkdirErr)
		}
	}

	offset := 0
	pageSize := 120
	for {
		library, fetchErr := zc.fetchLibrary(pageSize, offset)
		if fetchErr != nil {
			// Transient failure — wait and retry rather than aborting the whole run
			fmt.Printf("Library fetch failed: %v — waiting 60s before retry\n", fetchErr)
			time.Sleep(60 * time.Second)
			continue
		}
		if len(library.Data) == 0 {
			break
		}
		fmt.Printf("Fetched %d issues (offset %d)\n", len(library.Data), offset)

		for _, issue := range library.Data {
			pubName := RemoveBadCharacters(issue.Publication.Name)
			issueName := RemoveBadCharacters(issue.Name)
			completeName := filepath.Join(issueDirectory, pubName+" - "+issueName+".pdf")

			if fileExists(completeName) {
				fmt.Println("Already exists:", completeName)
				continue
			}

			fmt.Printf("Downloading: %s - %s\n", pubName, issueName)
			content := zc.fetchReaderContent(issue.Id)
			if len(content.Data.Pages) == 0 {
				fmt.Println("No pages found for issue", issue.Id)
				continue
			}

			legacyHash := content.Data.Issue.LegacyHash
			hash := content.Data.Issue.Hash

			issuePath := filepath.Join(issueDirectory, strconv.Itoa(issue.Id))

			// Build deduplicated password list to try: legacy_hash → hash → unencrypted
			seen := map[string]bool{}
			var uniquePasswords []string
			for _, pw := range []string{legacyHash, hash, ""} {
				if !seen[pw] {
					seen[pw] = true
					uniquePasswords = append(uniquePasswords, pw)
				}
			}

			var filenames []string
			for _, page := range content.Data.Pages {
				if page.Src == "" {
					continue
				}
				encPath := issuePath + "_" + page.Index + "_enc.pdf"
				decPath := issuePath + "_" + page.Index + ".pdf"

				resp, dlErr := zc.http.Get(page.Src)
				if dlErr != nil {
					fmt.Printf("Failed to download page %s: %s\n", page.Index, dlErr)
					continue
				}
				if resp.StatusCode != http.StatusOK {
					resp.Body.Close()
					fmt.Printf("Non-200 response for page %s: %d\n", page.Index, resp.StatusCode)
					continue
				}
				data, readErr := ioutil.ReadAll(resp.Body)
				resp.Body.Close()
				if readErr != nil {
					fmt.Printf("Failed to read page %s: %s\n", page.Index, readErr)
					continue
				}
				ioutil.WriteFile(encPath, data, 0644)

				decrypted := false
				for _, pw := range uniquePasswords {
					conf := model.NewAESConfiguration(pw, pw, 256)
					if decErr := api.DecryptFile(encPath, decPath, conf); decErr == nil {
						decrypted = true
						break
					}
				}
				if decrypted {
					os.Remove(encPath)
				} else {
					// PDF is not encrypted or uses unknown encryption — use as-is
					os.Rename(encPath, decPath)
				}
				filenames = append(filenames, decPath)
			}

			skipMerge := false
			for i := range filenames {
				if retryErr := retry(5, 2*time.Second, func() error {
					err := api.RemovePagesFile(filenames[i], "", []string{"2-"}, nil)
					if err != nil {
						fmt.Printf("Removing extra pages failed: %s\n", err)
					}
					return err
				}); retryErr != nil {
					fmt.Printf("Skipping merge for %s after repeated RemovePages failure: %s\n", completeName, retryErr)
					skipMerge = true
					break
				}
			}

			if !skipMerge {
				if mergeErr := api.MergeCreateFile(filenames, completeName, false, nil); mergeErr != nil {
					fmt.Printf("Merge failed for %s: %s\n", completeName, mergeErr)
				} else {
					fmt.Println("Saved:", completeName)
				}
			}

			for _, fileName := range filenames {
				os.Remove(fileName)
			}
		}

		offset += len(library.Data)
		if len(library.Data) < pageSize {
			break
		}
	}

	fmt.Println("Done.")
}

func login(client *http.Client, username, password, fingerprint string, newsstandID int) (LoginResponse, error) {
	fmt.Println("Logging in...")

	payload := map[string]interface{}{
		"email":    username,
		"password": password,
		"device": map[string]string{
			"name":        "Windows Chrome",
			"fingerprint": fingerprint,
			"device_type": "Desktop",
			"client_type": "Web",
		},
		"newsstand": map[string]interface{}{
			"currency": "USD", "id": newsstandID, "country": "US", "name": "United States",
			"cc": "us", "localeCode": "en_US", "userLang": "en_US", "userCountry": "US",
			"userCurrency": "USD",
		},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return LoginResponse{}, fmt.Errorf("failed to marshal login payload: %w", err)
	}

	req, err := http.NewRequest("POST", zinioBase+"/api/x7b9q-sync", bytes.NewBuffer(body))
	if err != nil {
		return LoginResponse{}, fmt.Errorf("failed to create login request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36")
	req.Header.Set("Origin", zinioBase)
	req.Header.Set("Referer", zinioBase+"/sign-in")

	resp, err := client.Do(req)
	if err != nil {
		return LoginResponse{}, fmt.Errorf("login request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return LoginResponse{}, fmt.Errorf("login returned HTTP %d", resp.StatusCode)
	}

	data, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return LoginResponse{}, fmt.Errorf("failed to read login response: %w", err)
	}

	var result LoginResponse
	if err := json.Unmarshal(data, &result); err != nil {
		return LoginResponse{}, fmt.Errorf("failed to parse login response: %w", err)
	}
	return result, nil
}

func (z *ZinioClient) fetchLibrary(limit, offset int) (LibraryResponse, error) {
	for attempt := 0; attempt < 2; attempt++ {
		u := fmt.Sprintf("%s/api/newsstand/newsstands/%d/users/%s/library-issues?limit=%d&offset=%d&sort=desc",
			zinioBase, z.newsstandID, z.userID, limit, offset)

		req, err := http.NewRequest("GET", u, nil)
		if err != nil {
			return LibraryResponse{}, fmt.Errorf("failed to create library request: %w", err)
		}
		req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
		req.Header.Set("Origin", zinioBase)

		resp, err := z.http.Do(req)
		if err != nil {
			return LibraryResponse{}, fmt.Errorf("library request failed: %w", err)
		}

		data, readErr := ioutil.ReadAll(resp.Body)
		resp.Body.Close()
		if readErr != nil {
			return LibraryResponse{}, fmt.Errorf("failed to read library response: %w", readErr)
		}

		if resp.StatusCode == 401 && attempt == 0 {
			fmt.Println("Library 401, re-authenticating...")
			if !z.relogin() {
				return LibraryResponse{}, fmt.Errorf("re-authentication failed")
			}
			continue
		}

		if resp.StatusCode != http.StatusOK {
			return LibraryResponse{}, fmt.Errorf("library returned HTTP %d", resp.StatusCode)
		}

		fmt.Println("Library response:", string(data)[:min(len(string(data)), 200)])
		var result LibraryResponse
		if err := json.Unmarshal(data, &result); err != nil {
			return LibraryResponse{}, fmt.Errorf("failed to parse library response: %w", err)
		}
		return result, nil
	}
	return LibraryResponse{}, fmt.Errorf("library fetch failed after re-authentication")
}

func (z *ZinioClient) fetchReaderContent(issueID int) ReaderContent {
	for attempt := 0; attempt < 2; attempt++ {
		u := fmt.Sprintf("%s/api/reader/content?issue_id=%d&newsstand_id=%d&user_id=%s",
			zinioBase, issueID, z.newsstandID, url.QueryEscape(z.userID))

		req, err := http.NewRequest("GET", u, nil)
		if err != nil {
			fmt.Printf("Failed to create reader request for issue %d: %v\n", issueID, err)
			return ReaderContent{}
		}
		req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
		req.Header.Set("Origin", zinioBase)

		resp, err := z.http.Do(req)
		if err != nil {
			fmt.Printf("Reader content fetch failed for issue %d: %v\n", issueID, err)
			return ReaderContent{}
		}

		data, readErr := ioutil.ReadAll(resp.Body)
		resp.Body.Close()
		if readErr != nil {
			fmt.Printf("Failed to read reader response for issue %d: %v\n", issueID, readErr)
			return ReaderContent{}
		}

		if resp.StatusCode == 401 && attempt == 0 {
			fmt.Println("Reader 401, re-authenticating...")
			if !z.relogin() {
				return ReaderContent{}
			}
			continue
		}

		if resp.StatusCode != http.StatusOK {
			fmt.Printf("Reader returned HTTP %d for issue %d\n", resp.StatusCode, issueID)
			return ReaderContent{}
		}

		var result ReaderContent
		if err := json.Unmarshal(data, &result); err != nil {
			fmt.Printf("Failed to parse reader response for issue %d: %v\n", issueID, err)
			return ReaderContent{}
		}
		fmt.Printf("Issue %d: %d pages\n", issueID, len(result.Data.Pages))
		return result
	}
	return ReaderContent{}
}

func fileExists(filename string) bool {
	info, err := os.Stat(filename)
	if os.IsNotExist(err) {
		return false
	}
	if os.IsPermission(err) {
		fmt.Println("Permission denied:", filename)
		return true
	}
	return !info.IsDir()
}

func retry(attempts int, sleep time.Duration, f func() error) error {
	for i := 0; ; i++ {
		err := f()
		if err == nil {
			return nil
		}
		if i >= attempts-1 {
			return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
		}
		time.Sleep(sleep)
		fmt.Println("retrying after error:", err)
	}
}

var badCharacters = []string{"/", "\\", "<", ">", ":", "\"", "|", "?", "*"}

func RemoveBadCharacters(input string) string {
	temp := input
	for _, badChar := range badCharacters {
		temp = strings.Replace(temp, badChar, "_", -1)
	}
	return stringsx.Clean(temp)
}

var letters = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ1234567890")

func randSeq(n int) string {
	rand.Seed(time.Now().UnixNano())
	b := make([]rune, n)
	for i := range b {
		b[i] = letters[rand.Intn(len(letters))]
	}
	return string(b)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
