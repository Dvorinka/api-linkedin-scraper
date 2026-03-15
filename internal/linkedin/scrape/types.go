package scrape

type ProfileInput struct {
	URL          string `json:"url"`
	IncludePosts bool   `json:"include_posts,omitempty"`
	Limit        int    `json:"limit,omitempty"`
}

type CompanyInput struct {
	URL          string `json:"url"`
	IncludePosts bool   `json:"include_posts,omitempty"`
	Limit        int    `json:"limit,omitempty"`
}

type Post struct {
	Text        string `json:"text"`
	URL         string `json:"url,omitempty"`
	PublishedAt string `json:"published_at,omitempty"`
}

type ProfileResult struct {
	URL             string `json:"url"`
	FullName        string `json:"full_name,omitempty"`
	Headline        string `json:"headline,omitempty"`
	Location        string `json:"location,omitempty"`
	About           string `json:"about,omitempty"`
	ProfileImageURL string `json:"profile_image_url,omitempty"`
	Posts           []Post `json:"posts,omitempty"`
}

type CompanyResult struct {
	URL          string `json:"url"`
	Name         string `json:"name,omitempty"`
	Industry     string `json:"industry,omitempty"`
	Size         string `json:"size,omitempty"`
	Website      string `json:"website,omitempty"`
	Headquarters string `json:"headquarters,omitempty"`
	About        string `json:"about,omitempty"`
	LogoURL      string `json:"logo_url,omitempty"`
	Posts        []Post `json:"posts,omitempty"`
}
