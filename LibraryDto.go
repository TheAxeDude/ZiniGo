package main

type LibraryResponse struct {
	Status bool   `json:"status"`
	Data  []LibraryData `json:"data"`
}

type LibraryData struct {
	Id int `json:"id"`
	Name string `json:"name"`
	Publication Publication `json:"publication"`
}

type Publication struct {
	Name string `json:"name"`

}