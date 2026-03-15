package scrape

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestScrapeProfile(t *testing.T) {
	profileHTML := `
	<html><head>
	<script type="application/ld+json">
	{
	  "@context":"https://schema.org",
	  "@type":"Person",
	  "name":"Jane Doe",
	  "jobTitle":"Senior Go Engineer",
	  "description":"Building resilient API platforms.",
	  "address":{"@type":"PostalAddress","addressLocality":"Prague","addressCountry":"CZ"},
	  "image":"https://cdn.example.com/jane.jpg"
	}
	</script>
	<script type="application/ld+json">
	[{"@type":"SocialMediaPosting","headline":"Post A","url":"https://www.linkedin.com/posts/jane-a","datePublished":"2026-02-20"},{"@type":"SocialMediaPosting","headline":"Post B","url":"https://www.linkedin.com/posts/jane-b","datePublished":"2026-02-21"}]
	</script>
	</head><body></body></html>`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/in/jane-doe" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(profileHTML))
	}))
	defer server.Close()

	svc := NewService(server.URL)
	result, err := svc.ScrapeProfile(context.Background(), ProfileInput{
		URL:          "https://www.linkedin.com/in/jane-doe",
		IncludePosts: true,
		Limit:        1,
	})
	if err != nil {
		t.Fatalf("scrape profile failed: %v", err)
	}
	if result.FullName != "Jane Doe" {
		t.Fatalf("unexpected full_name: %s", result.FullName)
	}
	if result.Headline == "" {
		t.Fatalf("expected headline")
	}
	if len(result.Posts) != 1 {
		t.Fatalf("expected 1 post, got %d", len(result.Posts))
	}
}

func TestScrapeCompany(t *testing.T) {
	companyHTML := `
	<html><head>
	<script type="application/ld+json">
	{
	  "@context":"https://schema.org",
	  "@type":"Organization",
	  "name":"Acme AI",
	  "industry":"Software Development",
	  "numberOfEmployees":{"minValue":"50","maxValue":"200"},
	  "url":"https://acme.example",
	  "description":"AI platform for B2B automation.",
	  "logo":"https://cdn.example.com/acme.png",
	  "address":{"@type":"PostalAddress","addressLocality":"Brno","addressCountry":"CZ"}
	}
	</script>
	</head><body></body></html>`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/company/acme-ai" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(companyHTML))
	}))
	defer server.Close()

	svc := NewService(server.URL)
	result, err := svc.ScrapeCompany(context.Background(), CompanyInput{
		URL: "https://www.linkedin.com/company/acme-ai",
	})
	if err != nil {
		t.Fatalf("scrape company failed: %v", err)
	}
	if result.Name != "Acme AI" {
		t.Fatalf("unexpected name: %s", result.Name)
	}
	if result.Industry == "" {
		t.Fatalf("expected industry")
	}
	if result.Size == "" {
		t.Fatalf("expected size")
	}
}

func TestNormalizeLinkedInPath(t *testing.T) {
	path, fullURL, err := normalizeLinkedInPath("https://www.linkedin.com/in/jane-doe", true)
	if err != nil {
		t.Fatalf("normalize profile failed: %v", err)
	}
	if path != "/in/jane-doe" || fullURL != "https://www.linkedin.com/in/jane-doe" {
		t.Fatalf("unexpected normalize output: %s %s", path, fullURL)
	}

	_, _, err = normalizeLinkedInPath("https://www.linkedin.com/company/acme", true)
	if err == nil {
		t.Fatalf("expected profile path validation error")
	}

	_, _, err = normalizeLinkedInPath("/company/acme", false)
	if err != nil {
		t.Fatalf("normalize company failed: %v", err)
	}
}
