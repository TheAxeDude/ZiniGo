package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/pdfcpu/pdfcpu/pkg/api"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
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

	flag.Parse()

	fmt.Println("Starting the application...")
	initialToken, err := GetInitialToken()
	if err != nil {
		os.Exit(1)
	}
	loginToken := GetLoginToken(initialToken, *usernamePtr, *passwordPtr)
	issues := GetLibrary(loginToken)
	fmt.Println("Found " + strconv.Itoa(len(issues.Data))+ " issues in library.")

	fmt.Println("Loading HTML template")
	template, _ := ioutil.ReadFile("template.html")

	//fmt.Println("Grabbing list of pages...")
	for _, issue := range issues.Data {
		issuePath := "./issue/" + strconv.Itoa(issue.Id)

		completeName := "./issue/" + issue.Publication.Name + " - " + issue.Name + ".pdf"
		if fileExists(completeName) {
			fmt.Println("Issue already found: "+ issue.Publication.Name + " - " + issue.Name)
			continue
		}


		pages := GetPages(loginToken, issue)

		var filenames []string

		for i := 0; i < len(pages.Data); i++ {
			fmt.Println("Source ", pages.Data[i].Source)
			fmt.Println("ID: ", pages.Data[i].Index)

			pathString := issuePath + "_" + pages.Data[i].Index

			htmldata := strings.Replace(string(template), "SVG_PATH", pages.Data[i].Source, -1)
			//write html file, embedding svg
			ioutil.WriteFile(pathString  +".html", []byte(htmldata), 0644)

			//convert to pdf

			cmd := exec.Command(*chromePtr, "--headless", "--disable-gpu", "--print-to-pdf="+pathString+".pdf", pathString+".html")

			fmt.Println(cmd.Args)
			err := cmd.Run()
			if err != nil {
				fmt.Printf("cmd.Run() failed with %s\n. You should retry this page.", err)
			}

			_ = os.Remove(pathString + ".html")
			_ = os.Remove(pathString + ".svg")

			//remove last page
			_ = api.RemovePagesFile(pathString+".pdf", "", []string{"2"}, nil)
			filenames = append(filenames, pathString+".pdf")
		}

		_ = api.MergeCreateFile(filenames, completeName, nil)


		for _, fileName := range filenames{
			_ = os.Remove(fileName)
		}
	}


	//

	fmt.Println("Terminating the application...")
}

func GetPages(userToken LoginResponse, issue LibraryData) Response {

	client := &http.Client{
	}

	req, _ := http.NewRequest("GET", "https://api-sec-2.ziniopro.com/newsstand/v2/newsstands/101/issues/"+strconv.Itoa(issue.Id)+"/content/pages?format=svg&application_id=9901&css_content=true&user_id="+ userToken.Data.User.UserIDString, nil)

	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("Authorization", "bearer " + userToken.Data.Token.AccessToken)

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

func GetLoginToken(initialToken string, username string, password string) LoginResponse{
	client := &http.Client{
	}

	var jsonStr = []byte(`{"username":"`+username+`","password":"`+password+`"}`)
	req, _ := http.NewRequest("POST", "https://www.zinio.com/api/login?project=99&logger=null", bytes.NewBuffer(jsonStr))

	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("Authorization", initialToken)

	resp, _ := client.Do(req)
	data, _ := ioutil.ReadAll(resp.Body)

	responseType := LoginResponse{}

	_ = json.Unmarshal([]byte(data), &responseType)

	return responseType

}

func GetLibrary(userToken LoginResponse) LibraryResponse{
	client := &http.Client{
	}

	req, _ := http.NewRequest("GET", "https://api-sec-2.ziniopro.com/newsstand/v2/newsstands/101/users/"+ userToken.Data.User.UserIDString+"/library_issues", nil)

	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("Authorization", "bearer " + userToken.Data.Token.AccessToken)

	resp, _ := client.Do(req)
	data, _ := ioutil.ReadAll(resp.Body)

	responseType := LibraryResponse{}

	_ = json.Unmarshal(data, &responseType)

	return responseType
}

func fileExists(filename string) bool {
	info, err := os.Stat(filename)
	if os.IsNotExist(err) {
		return false
	}
	return !info.IsDir()
}