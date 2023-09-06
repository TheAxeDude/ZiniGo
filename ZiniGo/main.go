package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/icza/gox/stringsx"
	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
	"io/ioutil"
	"log"
	"math/rand"
	"net/http"
	"os"
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
	//exportUsingWKHTML := flag.String("wkhtml", "false", "Use WKHTML instead of Chrome to generate PDF (false by default)")
	//exportUsingPlaywright := flag.String("playwright", "false", "Use Playwright Chromium instead of local Chrome to generate PDF (false by default)")
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

			issueDetails := GetIssueDetails(loginToken, issue.Id)
			isLegacy := issueDetails.Data.Issue.Publication.LegacyContent == 1
			passwordToUse := issueDetails.Data.Issue.Hash
			if isLegacy {
				passwordToUse = issueDetails.Data.Issue.LegacyHash
			}
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
			conf := pdfcpu.NewAESConfiguration(passwordToUse, passwordToUse, 256)
			for i := 0; i < len(pages.Data); i++ {
				if len(pages.Data[i].Src) == 0 {

					fmt.Println("No Download URL for page ", i)
					continue
				}
				fmt.Println("Source ", pages.Data[i].Src)
				fmt.Println("ID: ", pages.Data[i].Index)

				pathString := issuePath + "_" + pages.Data[i].Index

				resp, err := http.Get(pages.Data[i].Src)
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
				ioutil.WriteFile(pathString+".pdf", html, 0644)
				api.DecryptFile(pathString+".pdf", "", conf)

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

func GetIssueDetails(userToken LoginResponse, id int) IssueDetails {
	client := &http.Client{}

	req, _ := http.NewRequest("GET", "https://www.zinio.com/api/reader/content?issue_id="+strconv.Itoa(id)+"&newsstand_id=101&user_id="+userToken.Data.User.UserIDString+"&format=pdf&project=99&logger=null", nil)

	req.Header.Add("Content-Type", "application/json")
	for _, cookie := range userToken.Data.Cookies {
		req.AddCookie(cookie)

	}

	resp, _ := client.Do(req)
	data, _ := ioutil.ReadAll(resp.Body)

	responseType := IssueDetails{}

	_ = json.Unmarshal([]byte(data), &responseType)

	return responseType
}

func GetPages(userToken LoginResponse, issue LibraryData, endpoint string) AutoGenerated {

	client := &http.Client{}

	req, _ := http.NewRequest("GET", "https://zinio.com/api/newsstand/newsstands/101/issues/"+strconv.Itoa(issue.Id)+"/content/pages?format=pdf&application_id=9901&css_content=true&user_id="+userToken.Data.User.UserIDString, nil)

	req.Header.Add("Content-Type", "application/json")
	for _, cookie := range userToken.Data.Cookies {
		req.AddCookie(cookie)

	}

	resp, _ := client.Do(req)
	data, _ := ioutil.ReadAll(resp.Body)

	responseType := AutoGenerated{}

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
	LegacyHash  string      `json:"legacy_hash"`
	Hash        string      `json:"hash"`
}

type Publication struct {
	Name          string `json:"name"`
	LegacyContent int    `json:"legacy_content"`
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

type IssueDetails struct {
	Data struct {
		Issue struct {
			ID                   int    `json:"id"`
			PublicationID        int    `json:"publication_id"`
			Name                 string `json:"name"`
			InternalName         string `json:"internal_name"`
			Issn                 string `json:"issn"`
			VolumeNo             string `json:"volume_no"`
			IssueNo              string `json:"issue_no"`
			SequenceNo           string `json:"sequence_no"`
			Description          string `json:"description"`
			Slug                 string `json:"slug"`
			Code                 string `json:"code"`
			CoverImage           string `json:"cover_image"`
			CoverDate            string `json:"cover_date"`
			PublishDate          string `json:"publish_date"`
			PublishEffectiveDate string `json:"publish_effective_date"`
			RemoteIdentifier     string `json:"remote_identifier"`
			LegacyIssueID        int    `json:"legacy_issue_id"`
			LegacyIdentifier     string `json:"legacy_identifier"`
			Status               int    `json:"status"`
			CreatedAt            string `json:"created_at"`
			ModifiedAt           string `json:"modified_at"`
			CreatedBy            int    `json:"created_by"`
			ModifiedBy           int    `json:"modified_by"`
			FilePath             string `json:"file_path"`
			Type                 int    `json:"type"`
			Preview              int    `json:"preview"`
			HasXML               int    `json:"has_xml"`
			HasPdf               int    `json:"has_pdf"`
			Binding              int    `json:"binding"`
			FulfilmentCode       string `json:"fulfilment_code"`
			AllowPrinting        int    `json:"allow_printing"`
			Watermark            int    `json:"watermark"`
			CoverPrice           int    `json:"cover_price"`
			CoverCurrency        string `json:"cover_currency"`
			NoOfPages            int    `json:"no_of_pages"`
			Classification       int    `json:"classification"`
			ContentRevision      any    `json:"content_revision"`
			Publication          struct {
				ID                         int    `json:"id"`
				Name                       string `json:"name"`
				Frequency                  string `json:"frequency"`
				LegacyIdentifier           string `json:"legacy_identifier"`
				InternalName               string `json:"internal_name"`
				Description                string `json:"description"`
				PublisherID                int    `json:"publisher_id"`
				ContentRating              int    `json:"content_rating"`
				RemoteIdentifier           string `json:"remote_identifier"`
				CreatedAt                  string `json:"created_at"`
				ModifiedAt                 string `json:"modified_at"`
				CreatedBy                  int    `json:"created_by"`
				ModifiedBy                 int64  `json:"modified_by"`
				SiteID                     int    `json:"site_id"`
				Status                     int    `json:"status"`
				Type                       int    `json:"type"`
				NoOfIssues                 int    `json:"no_of_issues"`
				Logo                       string `json:"logo"`
				AllowXML                   int    `json:"allow_xml"`
				AllowPdf                   int    `json:"allow_pdf"`
				LatinName                  string `json:"latin_name"`
				Tagline                    string `json:"tagline"`
				ParentID                   any    `json:"parent_id"`
				SeoKeywords                any    `json:"seo_keywords"`
				SearchKeywords             string `json:"search_keywords"`
				Issn                       string `json:"issn"`
				CirculationType            int    `json:"circulation_type"`
				Binding                    int    `json:"binding"`
				Watermark                  int    `json:"watermark"`
				AllowPrinting              int    `json:"allow_printing"`
				AllowIntegratedFulfilment  int    `json:"allow_integrated_fulfilment"`
				FulfilmentHouseID          string `json:"fulfilment_house_id"`
				FulfilmentCode             string `json:"fulfilment_code"`
				DefaultCurrencyCode        string `json:"default_currency_code"`
				Slug                       string `json:"slug"`
				SourceType                 int    `json:"source_type"`
				LegacyContent              int    `json:"legacy_content"`
				HasToGetPreviousIssuePrice any    `json:"has_to_get_previous_issue_price"`
				Country                    struct {
					Format string `json:"format"`
					Name   string `json:"name"`
					Code   string `json:"code"`
				} `json:"country"`
				Locale struct {
					Format string `json:"format"`
					Name   string `json:"name"`
					Code   string `json:"code"`
				} `json:"locale"`
				Language struct {
					Format string `json:"format"`
					Name   string `json:"name"`
					Code   string `json:"code"`
				} `json:"language"`
				Publisher struct {
					ID               string `json:"id"`
					Name             string `json:"name"`
					InternalName     string `json:"internal_name"`
					Description      any    `json:"description"`
					Slug             any    `json:"slug"`
					Code             any    `json:"code"`
					Logo             any    `json:"logo"`
					RemoteIdentifier any    `json:"remote_identifier"`
					Status           int    `json:"status"`
					Country          struct {
						Format string `json:"format"`
						Name   string `json:"name"`
						Code   string `json:"code"`
					} `json:"country"`
				} `json:"publisher"`
			} `json:"publication"`
			Metadata []any `json:"metadata"`
			Product  struct {
				ID               int    `json:"id"`
				Code             string `json:"code"`
				Type             int    `json:"type"`
				Rrp              any    `json:"rrp"`
				RrpCurrencyCode  any    `json:"rrp_currency_code"`
				Name             string `json:"name"`
				Description      any    `json:"description"`
				RemoteIdentifier any    `json:"remote_identifier"`
				LegacyID         any    `json:"legacy_id"`
				ProjectID        any    `json:"project_id"`
				PublicationID    int    `json:"publication_id"`
				IssueID          int    `json:"issue_id"`
				CatalogID        any    `json:"catalog_id"`
				TermAmount       any    `json:"term_amount"`
				TermUnits        any    `json:"term_units"`
				SaleTier         int    `json:"sale_tier"`
				Credits          any    `json:"credits"`
				Duration         any    `json:"duration"`
				Status           int    `json:"status"`
				AvailabilityDate string `json:"availability_date"`
				CreatedAt        string `json:"created_at"`
				ModifiedAt       string `json:"modified_at"`
			} `json:"product"`
			Prices []struct {
				ID                                  int    `json:"id"`
				PublicationID                       int    `json:"publication_id"`
				ProjectID                           int    `json:"project_id"`
				NewsstandID                         any    `json:"newsstand_id"`
				ProductType                         int    `json:"product_type"`
				DefaultProduct                      int    `json:"default_product"`
				ProductID                           any    `json:"product_id"`
				IssueID                             any    `json:"issue_id"`
				SaleTier                            any    `json:"sale_tier"`
				IssueType                           any    `json:"issue_type"`
				Country                             any    `json:"country"`
				Price                               any    `json:"price"`
				TaxInclusivePrice                   any    `json:"tax_inclusive_price"`
				PriceAfterCoupon                    any    `json:"price_after_coupon"`
				TaxInclusivePriceAfterCoupon        any    `json:"tax_inclusive_price_after_coupon"`
				Coupon                              any    `json:"coupon"`
				Currency                            any    `json:"currency"`
				Tier                                string `json:"tier"`
				DistributionPlatform                int    `json:"distribution_platform"`
				ReferencePriceID                    int    `json:"reference_price_id"`
				TaxRate                             any    `json:"tax_rate"`
				ExchangeRate                        any    `json:"exchange_rate"`
				CreatedBy                           int    `json:"created_by"`
				ModifiedBy                          any    `json:"modified_by"`
				CreatedAt                           string `json:"created_at"`
				ModifiedAt                          string `json:"modified_at"`
				Sku                                 string `json:"sku"`
				DisplayPrice                        any    `json:"display_price"`
				TaxInclusiveDisplayPrice            any    `json:"tax_inclusive_display_price"`
				DisplayPriceAfterCoupon             any    `json:"display_price_after_coupon"`
				TaxInclusiveDisplayPriceAfterCoupon any    `json:"tax_inclusive_display_price_after_coupon"`
				DisplayCurrency                     any    `json:"display_currency"`
			} `json:"prices"`
			AllowXML   int    `json:"allow_xml"`
			AllowPdf   int    `json:"allow_pdf"`
			LegacyHash string `json:"legacy_hash"`
			Hash       string `json:"hash"`
		} `json:"issue"`
		Pages []struct {
			Index       string `json:"index"`
			FolioNumber string `json:"folio_number"`
			Src         string `json:"src"`
			Checksum    string `json:"checksum"`
			Preview     string `json:"preview"`
			Type        string `json:"type"`
			PdfTag      string `json:"pdf_tag"`
			Mine        string `json:"mine"`
			Width       int    `json:"width"`
			Height      int    `json:"height"`
			Links       []any  `json:"links"`
			Thumbnail   string `json:"thumbnail"`
		} `json:"pages"`
		Stories []struct {
			ID            int       `json:"id"`
			UniqueStoryID string    `json:"unique_story_id"`
			Title         string    `json:"title"`
			SubTitle      string    `json:"sub_title"`
			StrapLine     string    `json:"strap_line"`
			Intro         string    `json:"intro"`
			Authors       []any     `json:"authors"`
			Preview       int       `json:"preview"`
			Priority      int       `json:"priority"`
			Tag           string    `json:"tag"`
			StartingPage  string    `json:"starting_page"`
			PageRange     string    `json:"page_range"`
			ModifiedDate  time.Time `json:"modified_date"`
			Template      struct {
				ID     int      `json:"id"`
				Code   string   `json:"code"`
				Name   string   `json:"name"`
				CSS    []string `json:"css"`
				Fonts  []string `json:"fonts"`
				Images []any    `json:"images"`
			} `json:"template"`
			Content      string `json:"content"`
			FeatureImage string `json:"feature_image"`
			Images       []any  `json:"images"`
			Section      struct {
				ID          int    `json:"id"`
				Name        string `json:"name"`
				Description string `json:"description"`
				Priority    int    `json:"priority"`
			} `json:"section"`
			Excerpt        string `json:"excerpt"`
			ManualTags     []any  `json:"manual_tags"`
			RelatedObjects struct {
				Image []any `json:"image"`
			} `json:"related_objects"`
		} `json:"stories"`
		Ads []struct {
			ID               string `json:"id"`
			UniqueStoryID    string `json:"unique_story_id"`
			AdvertiseCode    string `json:"advertise_code"`
			RelativeObjectID string `json:"relative_object_id"`
			RelativeRemoteID string `json:"relative_remote_id"`
			Priority         string `json:"priority"`
			Position         string `json:"position"`
			Folio            string `json:"folio"`
			Version          string `json:"version"`
			RemoteID         string `json:"remote_id"`
			AdsType          string `json:"ads_type"`
			IssuePdfImageAds struct {
				LocalFileURL    string `json:"local_file_url"`
				Portrait        string `json:"portrait"`
				Landscape       string `json:"landscape"`
				ClickthroughURL string `json:"clickthrough_url"`
				Checksum        string `json:"checksum"`
			} `json:"issue_pdf_image_ads"`
			Created   string `json:"created"`
			CreatedBy string `json:"created_by"`
		} `json:"ads"`
		Entitlement struct {
			DeliveryID       any       `json:"delivery_id"`
			LegacyIdentifier any       `json:"legacy_identifier"`
			Type             int       `json:"type"`
			ID               int64     `json:"id"`
			UserID           int64     `json:"user_id"`
			DeviceID         int       `json:"device_id"`
			IssueID          int       `json:"issue_id"`
			PublicationID    int       `json:"publication_id"`
			ProjectID        int       `json:"project_id"`
			OrderID          int       `json:"order_id"`
			LabelID          int64     `json:"label_id"`
			Status           int       `json:"status"`
			Archived         bool      `json:"archived"`
			ArchivedStatus   int       `json:"archived_status"`
			CreatedAt        time.Time `json:"created_at"`
			ArchivedAt       any       `json:"archived_at"`
			ModifiedAt       time.Time `json:"modified_at"`
		} `json:"entitlement"`
	} `json:"data"`
}

type AutoGenerated struct {
	Status bool `json:"status"`
	Data   []struct {
		Index       string `json:"index"`
		FolioNumber string `json:"folio_number"`
		Src         string `json:"src"`
		Checksum    string `json:"checksum"`
		Preview     string `json:"preview"`
		Type        string `json:"type"`
		PdfTag      string `json:"pdf_tag"`
		Mine        string `json:"mine"`
		Width       int    `json:"width"`
		Height      int    `json:"height"`
		Links       []any  `json:"links"`
		Thumbnail   string `json:"thumbnail"`
	} `json:"data"`
}
