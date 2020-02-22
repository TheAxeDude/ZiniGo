package LibraryDto

type Response struct {
	Status bool   `json:"status"`
	Data  []Data `json:"data"`
}

type Data struct {
	Id int `json:"id"`
	Name string `json:"name"`
	Publication Publication `json:"publication"`
}

type Publication struct {
	Name string `json:"name"`

}