package services

import (
	"encoding/csv"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Ulzuhan/linkup/internal/models"
)

type BulkImportResult struct {
	TotalProcessed int      `json:"total_processed"`
	TotalCreated   int      `json:"total_created"`
	TotalSkipped   int      `json:"total_skipped"`
	Errors         []string `json:"errors"`
}

type CSVService struct {
	linkService *LinkService
}

func NewCSVService(linkService *LinkService) *CSVService {
	return &CSVService{linkService: linkService}
}

// ImportCSV parses a CSV reader and creates short links in parallel
func (s *CSVService) ImportCSV(reader io.Reader, username string) (*BulkImportResult, error) {
	csvReader := csv.NewReader(reader)
	csvReader.TrimLeadingSpace = true
	csvReader.FieldsPerRecord = -1 // flexible fields

	records, err := csvReader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("failed to read CSV: %w", err)
	}

	if len(records) < 2 {
		return nil, fmt.Errorf("CSV must contain a header row and at least one data row")
	}

	// Map headers
	headers := records[0]
	headerMap := make(map[string]int)
	for i, h := range headers {
		headerMap[strings.ToLower(strings.TrimSpace(h))] = i
	}

	urlIdx, hasURL := headerMap["url"]
	if !hasURL {
		urlIdx, hasURL = headerMap["target_url"]
	}
	if !hasURL {
		urlIdx, hasURL = headerMap["destination"]
	}
	if !hasURL {
		return nil, fmt.Errorf("CSV is missing a required 'url' or 'target_url' column")
	}

	slugIdx, hasSlug := headerMap["slug"]
	if !hasSlug {
		slugIdx, hasSlug = headerMap["custom_slug"]
	}
	titleIdx, hasTitle := headerMap["title"]
	pinIdx, hasPIN := headerMap["pin"]
	maxClicksIdx, hasMaxClicks := headerMap["max_clicks"]

	dataRows := records[1:]
	result := &BulkImportResult{
		TotalProcessed: len(dataRows),
	}

	var mu sync.Mutex
	var wg sync.WaitGroup
	semaphore := make(chan struct{}, 10) // Limit concurrency to 10 workers

	for lineNum, row := range dataRows {
		if len(row) <= urlIdx {
			continue
		}

		rawURL := strings.TrimSpace(row[urlIdx])
		if rawURL == "" {
			continue
		}

		var customSlug, title, pin string
		var maxClicks *int

		if hasSlug && len(row) > slugIdx {
			customSlug = strings.TrimSpace(row[slugIdx])
		}
		if hasTitle && len(row) > titleIdx {
			title = strings.TrimSpace(row[titleIdx])
		}
		if hasPIN && len(row) > pinIdx {
			pin = strings.TrimSpace(row[pinIdx])
		}
		if hasMaxClicks && len(row) > maxClicksIdx {
			if val, err := strconv.Atoi(strings.TrimSpace(row[maxClicksIdx])); err == nil && val > 0 {
				maxClicks = &val
			}
		}

		wg.Add(1)
		semaphore <- struct{}{}

		go func(rowNum int, req models.CreateLinkRequest) {
			defer wg.Done()
			defer func() { <-semaphore }()

			_, _, err := s.linkService.Create(req, username)
			mu.Lock()
			defer mu.Unlock()

			if err != nil {
				result.TotalSkipped++
				result.Errors = append(result.Errors, fmt.Sprintf("Row %d: %v", rowNum+2, err))
			} else {
				result.TotalCreated++
			}
		}(lineNum, models.CreateLinkRequest{
			URL:          rawURL,
			CustomSlug:   customSlug,
			Title:        title,
			PIN:          pin,
			MaxClicks:    maxClicks,
			RedirectType: 302,
		})
	}

	wg.Wait()
	return result, nil
}

// ExportCSV streams user links as a standard CSV format
func (s *CSVService) ExportCSV(w io.Writer, username string, isAdmin bool) error {
	links, err := s.linkService.ListByUser(username, isAdmin)
	if err != nil {
		return err
	}

	csvWriter := csv.NewWriter(w)
	defer csvWriter.Flush()

	// Write Header
	headers := []string{"ID", "Slug", "Domain", "Target URL", "Original URL", "Title", "Clicks", "Max Clicks", "Has PIN", "Status", "Created At"}
	if err := csvWriter.Write(headers); err != nil {
		return err
	}

	for _, l := range links {
		status := "active"
		if l.IsExpired() {
			status = "expired"
		}

		maxClicksStr := ""
		if l.MaxClicks != nil {
			maxClicksStr = strconv.Itoa(*l.MaxClicks)
		}

		row := []string{
			l.ID,
			l.Slug,
			l.Domain,
			l.TargetURL,
			l.OriginalURL,
			l.Title,
			strconv.Itoa(l.ClickCount),
			maxClicksStr,
			strconv.FormatBool(l.HasPIN),
			status,
			time.Unix(l.CreatedAt, 0).Format(time.RFC3339),
		}

		if err := csvWriter.Write(row); err != nil {
			return err
		}
	}

	return nil
}
