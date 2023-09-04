package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	wkhtml "github.com/SebastiaanKlippert/go-wkhtmltopdf"
	"github.com/icza/gox/stringsx"
	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/playwright-community/playwright-go"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
	"io/ioutil"
	"log"
	"math/rand"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type Response struct {
	Status bool   `json:"status"`
	Data   []Data `json:"data"`
}

type Data struct {
	Source string `json:"src"`
	Index  string `json:"index"`
}

func main() {

	usernamePtr := flag.String("u", "", "Zinio Username")
	passwordPtr := flag.String("p", "", "Zinio Password")
	chromePtr := flag.String("c", "google-chrome", "Chrome executable")
	zinioHostPtr := flag.String("e", "api-sec.ziniopro.com", "Zinio Host (Excluding port and URI Scheme). Known: `api-sec`, `api-sec-2`")
	exportUsingWKHTML := flag.String("wkhtml", "false", "Use WKHTML instead of Chrome to generate PDF (false by default)")
	exportUsingPlaywright := flag.String("playwright", "false", "Use Playwright Chromium instead of local Chrome to generate PDF (false by default)")
	deviceFingerprintPtr := flag.String("fingerprint", "abcd123", "This devices fingerprint - presented to Zinio API")

	flag.Parse()

	mydir, err := os.Getwd()
	if err != nil {
		fmt.Println(err)
	}

	if fileExists(mydir + "/config.json") {
		fmt.Println("Config file loaded")
		byteValue, _ := ioutil.ReadFile("config.json")
		username := gjson.GetBytes(byteValue, "username")
		if username.Exists() {
			*usernamePtr = username.String()
			fmt.Println("Username taken from config file")
		}

		password := gjson.GetBytes(byteValue, "password")
		if password.Exists() {
			*passwordPtr = password.String()
			fmt.Println("password taken from config file")
		}

		chrome := gjson.GetBytes(byteValue, "chromepath")
		if chrome.Exists() {
			*chromePtr = chrome.String()
			fmt.Println("chromepath taken from config file")
		}

		fingerprint := gjson.GetBytes(byteValue, "fingerprint")
		if fingerprint.Exists() {
			*deviceFingerprintPtr = fingerprint.String()
			fmt.Println("Fingerprint taken from config file")
		} else {
			fmt.Println("No fingerprint found in text file, generating and writing")
			newJson, _ := sjson.Set(string(byteValue), "fingerprint", randSeq(15))

			err := ioutil.WriteFile("config.json", []byte(newJson), 0644)
			if err != nil {
				log.Fatalf("unable to write file: %v", err)
			}
		}

	}

	//fmt.Println("Starting the application...")
	//initialToken, err := GetInitialToken()
	//if err != nil {
	//	os.Exit(1)
	//}
	loginToken := GetLoginToken(*usernamePtr, *passwordPtr, *deviceFingerprintPtr)
	issues := GetLibrary(loginToken, *zinioHostPtr)
	for i := range issues {
		issueList := issues[i]
		//fmt.Println("Found " + strconv.Itoa(len(issues.Data)) + " issues in library.")

		fmt.Println("Loading HTML template")
		defaultTemplate := GetDefaultTemplate()
		template, _ := ioutil.ReadFile("template.html")

		if template == nil || len(template) == 0 {
			fmt.Println("template.html not found, or empty. using issue in template. Consider changing this if your files are cropped.")
			template = []byte(defaultTemplate)
		}

		fmt.Println("Resolved working directory to: " + mydir)
		//fmt.Println("Grabbing list of pages...")
		issueDirectory := filepath.Join(mydir, "issue")
		if _, err := os.Stat(issueDirectory); os.IsNotExist(err) {
			os.Mkdir(issueDirectory, 0600)
		}

		for _, issue := range issueList.Data {
			fmt.Println(issue)
			issuePath := filepath.Join(issueDirectory, strconv.Itoa(issue.Id))

			publicationName := RemoveBadCharacters(issue.Publication.Name)
			issueName := RemoveBadCharacters(issue.Name)

			completeName := filepath.Join(issueDirectory, publicationName+" - "+issueName+".pdf")
			fmt.Println("Checking if issue exists: " + completeName)
			if fileExists(completeName) {
				fmt.Println("Issue already found: " + completeName)
				continue
			}
			fmt.Println("Downloading issue: " + publicationName + " - " + issueName)

			pages := GetPages(loginToken, issue, *zinioHostPtr)

			var filenames []string

			for i := 0; i < len(pages.Data); i++ {
				if len(pages.Data[i].Source) == 0 {

					fmt.Println("No Download URL for page ", i)
					continue
				}
				fmt.Println("Source ", pages.Data[i].Source)
				fmt.Println("ID: ", pages.Data[i].Index)

				pathString := issuePath + "_" + pages.Data[i].Index

				resp, err := http.Get(pages.Data[i].Source)
				// handle the error if there is one
				if err != nil {
					panic(err)
				}
				// do this now so it won't be forgotten
				defer resp.Body.Close()
				// reads html as a slice of bytes
				html, err := ioutil.ReadAll(resp.Body)
				if err != nil {
					panic(err)
				}
				// show the HTML code as a string %s

				htmldata := strings.Replace(string(template), "SVG_PATH", string(html), -1)

				//convert to pdf

				if strings.ToLower(*exportUsingPlaywright) == "true" {
					pw, err := playwright.Run()
					if err != nil {
						log.Fatalf("could not start playwright: %v", err)
					}
					browser, err := pw.Chromium.Launch()
					if err != nil {
						log.Fatalf("could not launch browser: %v", err)
					}
					context, err := browser.NewContext()

					page, err := context.NewPage()
					_ = page.SetContent(htmldata, playwright.PageSetContentOptions{WaitUntil: playwright.WaitUntilStateNetworkidle})
					_, err = page.PDF(playwright.PagePdfOptions{
						Path: playwright.String(pathString + ".pdf"),
					})

				} else if strings.ToLower(*exportUsingWKHTML) == "true" {

					pdfg, err := wkhtml.NewPDFGenerator()
					if err != nil {
						return
					}
					pdfg.MarginBottom.Set(0)
					pdfg.MarginTop.Set(0)
					pdfg.MarginLeft.Set(0)
					pdfg.MarginRight.Set(0)
					pdfg.NoOutline.Set(true)
					//pdfg.PageSize.Set(wkhtml.PageSizeCustom)
					pdfg.AddPage(wkhtml.NewPageReader(strings.NewReader(htmldata)))

					// Create PDF document in internal buffer
					err = pdfg.Create()
					if err != nil {
						log.Fatal(err)
					}

					//Your Pdf Name
					err = pdfg.WriteFile(pathString + ".pdf")
					if err != nil {
						log.Fatal(err)
					}

				} else {
					//write html file, embedding svg
					ioutil.WriteFile(pathString+".html", []byte(htmldata), 0644)
					cmd := exec.Command(*chromePtr, "--headless", "--disable-gpu", "--print-to-pdf="+pathString+".pdf", "--no-margins", pathString+".html")
					fmt.Println(cmd.Args)
					err := cmd.Run()
					if err != nil {
						fmt.Printf("cmd.Run() failed with %s\n. You should retry this page.", err)
					}

				}

				_ = os.Remove(pathString + ".html")
				_ = os.Remove(pathString + ".svg")

				filenames = append(filenames, pathString+".pdf")
			}

			for i := range filenames {
				//remove last page
				err = retry(5, 2*time.Second, func() (err error) {
					err = api.RemovePagesFile(filenames[i], "", []string{"2-"}, nil)
					if err != nil {
						fmt.Printf("Removing extra pages failed with %s\n.", err)
					}

					return
				})
			}

			_ = api.MergeCreateFile(filenames, completeName, nil)

			for _, fileName := range filenames {
				_ = os.Remove(fileName)
			}
		}

	}

	fmt.Println("Terminating the application...")

}

func GetPages(userToken LoginResponse, issue LibraryData, endpoint string) Response {

	client := &http.Client{}

	//req, _ := http.NewRequest("GET", "https://"+endpoint+"/newsstand/v2/newsstands/134/issues/"+strconv.Itoa(issue.Id)+"/content/pages?format=svg&application_id=9901&css_content=true&user_id="+userToken.Data.User.UserIDString, nil)
	req, _ := http.NewRequest("GET", "https://zinio.com/api/newsstand/newsstands/101/issues/"+strconv.Itoa(issue.Id)+"/content/pages?format=svg&application_id=9901&css_content=true&user_id="+userToken.Data.User.UserIDString, nil)

	req.Header.Add("Content-Type", "application/json")
	//req.AddCookie(&http.Cookie{Name: "zwrt", Value: userToken.Data.AccessToken})
	for _, cookie := range userToken.Data.Cookies {
		req.AddCookie(cookie)

	}

	//req.Header.Add("Authorization", "bearer "+token)
	//req.Header.Add("Authorization", token)

	resp, _ := client.Do(req)
	data, _ := ioutil.ReadAll(resp.Body)

	responseType := Response{}

	_ = json.Unmarshal([]byte(data), &responseType)

	return responseType
}

func GetInitialToken() (token string, err error) {
	page, err := http.Get("https://www.zinio.com/za/sign-in")
	if err != nil {
		fmt.Println("Unable to get initial token: " + err.Error())
		return "", err
	}

	data, _ := ioutil.ReadAll(page.Body)

	re := regexp.MustCompile(`"(jwt)":"((\\"|[^"])*)"`)

	found := re.FindSubmatch(data)

	return string(found[2]), nil
}

func GetLoginToken(username string, password string, fingerprint string) LoginResponse {
	fmt.Println("GettingLogin")

	client := &http.Client{}

	var jsonStr = []byte(`{"email":"` + username + `","password":"` + password + `","device":{"name":"Windows Chrome","fingerprint":"` + fingerprint + `","device_type":"Desktop","client_type":"Web"},"newsstand":{"currency":"ZAR","id":134,"country":"ZA","name":"South Africa","cc":"za","localeCode":"en_ZA","userLang":"en_ZA","userCountry":"ZA","userCurrency":"ZAR","requiresCookies":true,"requiresExplicitConsent":true,"requiresAdultConfirmation":true,"adWords":{"id":1,"label":""},"isDefaultNewsstand":false}}`)
	fmt.Println(string(jsonStr))

	req, _ := http.NewRequest("POST", "https://www.zinio.com/api/login?project=99&logger=null", bytes.NewBuffer(jsonStr))

	req.Header.Add("Content-Type", "application/json")
	//req.Header.Add("Authorization", initialToken)

	resp, _ := client.Do(req)
	data, _ := ioutil.ReadAll(resp.Body)
	//fmt.Println(string(data))

	responseType := LoginResponse{}

	_ = json.Unmarshal([]byte(data), &responseType)

	for _, cookie := range resp.Cookies() {
		fmt.Println("cookie name:" + cookie.Name + "cookie value" + cookie.Value)
		if cookie.Name == "zwrt" {
			responseType.Data.AccessToken = cookie.Value
		}
		if cookie.Name == "zwrrt" {
			responseType.Data.RefreshToken = cookie.Value
		}
	}
	responseType.Data.Cookies = resp.Cookies()
	fmt.Println("GotLogin")

	return responseType

}

func GetLibrary(userToken LoginResponse, endpoint string) []LibraryResponse {
	fmt.Println("Fetching Library")
	client := &http.Client{}

	var itemsToReturn []LibraryResponse
	issuesToFetch := 120

	pageToFetch := 1
	for {
		fmt.Println("Fetching page:" + strconv.Itoa(pageToFetch))

		req, _ := http.NewRequest("GET", "https://zinio.com/api/newsstand/newsstands/101/users/"+userToken.Data.User.UserIDString+"/library_issues?limit="+strconv.Itoa(issuesToFetch)+"&page="+strconv.Itoa(pageToFetch), nil)

		req.Header.Add("Content-Type", "application/json")
		for _, cookie := range userToken.Data.Cookies {
			req.AddCookie(cookie)
		}
		//req.AddCookie(&http.Cookie{Name: "zwrt", Value: userToken.Data.AccessToken})
		//req.Header.Add("Authorization", "bearer "+userToken.Data.Token.AccessToken)
		//req.Header.Add("Authorization", initialToken)

		resp, err := client.Do(req)

		if err != nil {
			fmt.Println("Unable to get Library: " + err.Error())
		}

		data, _ := ioutil.ReadAll(resp.Body)
		fmt.Println(string(data))

		responseType := LibraryResponse{}

		_ = json.Unmarshal(data, &responseType)

		if len(responseType.Data) > 0 {
			itemsToReturn = append(itemsToReturn, responseType)
			pageToFetch++
		} else {
			break
		}
	}

	return itemsToReturn
}

func fileExists(filename string) bool {
	info, err := os.Stat(filename)
	if os.IsNotExist(err) {
		return false
	}

	if os.IsPermission(err) {
		fmt.Println("Unable to read location - check permissions: " + filename)
		return true
	}
	return !info.IsDir()
}

type LoginResponse struct {
	Status bool      `json:"status"`
	Data   LoginData `json:"data"`
}

type LoginData struct {
	User         User   `json:"user"`
	Token        Token  `json:"token"`
	RefreshToken string `json:"refreshToken"`
	AccessToken  string
	Cookies      []*http.Cookie
}

type User struct {
	UserIDString string `json:"user_id_string"`
}

type Token struct {
	AccessToken string `json:"access_token"`
}

type LibraryResponse struct {
	Status bool          `json:"status"`
	Data   []LibraryData `json:"data"`
}

type LibraryData struct {
	Id          int         `json:"id"`
	Name        string      `json:"name"`
	Publication Publication `json:"publication"`
}

type Publication struct {
	Name string `json:"name"`
}

// https://stackoverflow.com/questions/47606761/repeat-code-if-an-error-occured
func retry(attempts int, sleep time.Duration, f func() error) (err error) {
	for i := 0; ; i++ {
		err = f()
		if err == nil {
			return
		}

		if i >= (attempts - 1) {
			break
		}

		time.Sleep(sleep)

		fmt.Println("retrying after error:", err)
	}
	return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

func GetDefaultTemplate() string {
	return `<html>
	<head>
	<!--<style>
	@media all {
		@page { margin: 0px; }
		body { margin-top: 0cm;
		margin-left:auto;
	}


	}
	</style>-->
	<style>
		html, body {
		width:  fit-content;
		height: fit-content;
		margin:  0px;
		padding: 0px;
	}
	</style>

	<style id=page_style>
	@page { size: 100px 100px ; margin : 0px }
	</style>
	</head>
	<body>
	<object type="image/svg+xml" data="SVG_PATH</object>

	<script>
		window.onload = fixpage;

	function fixpage() {

		renderBlock = document.getElementsByTagName("html")[0];
		renderBlockInfo = window.getComputedStyle(renderBlock)

		// fix chrome page bug
		fixHeight = parseInt(renderBlockInfo.height) + 1 + "px"

		pageCss = "@page { size: " + renderBlockInfo.width + " " + fixHeight +" ; margin:0;}"
		document.getElementById("page_style").innerHTML = pageCss
	}
	</script>
	</body>


	</html>`
}

var badCharacters = []string{"/", "\\", "<", ">", ":", "\"", "|", "?", "*"}

func RemoveBadCharacters(input string) string {

	temp := input

	for _, badChar := range badCharacters {
		temp = strings.Replace(temp, badChar, "_", -1)
	}

	temp = stringsx.Clean(temp)

	return temp
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
