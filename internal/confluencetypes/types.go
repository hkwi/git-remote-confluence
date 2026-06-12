package confluencetypes

type Version struct {
	Number    int    `json:"number"`
	When      string `json:"when"`
	MinorEdit bool   `json:"minorEdit"`
	Message   string `json:"message"`
	By        User   `json:"by"`
}

type User struct {
	Type        string `json:"type"`
	Username    string `json:"username"`
	UserKey     string `json:"userKey"`
	AccountID   string `json:"accountId"`
	DisplayName string `json:"displayName"`
}
