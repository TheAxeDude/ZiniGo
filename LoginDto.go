package main

type LoginResponse struct {
	Status bool   `json:"status"`
	Data  LoginData `json:"data"`
}

type LoginData struct {
	User User `json:"user"`
	Token Token `json:"token"`
	RefreshToken string `json:"refreshToken"`
}

type User struct {
	UserIDString string `json:"user_id_string"`
}

type Token struct {
	AccessToken string `json:"access_token"`

}