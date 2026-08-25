package engine

import (
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/AvenalJ/yt-archiver/internal/logger"
)

type NFOMovie struct {
	XMLName       xml.Name    `xml:"movie"`
	Title         string      `xml:"title"`
	OriginalTitle string      `xml:"originaltitle"`
	SortTitle     string      `xml:"sorttitle"`
	Plot          string      `xml:"plot"`
	Outline       string      `xml:"outline"`
	Runtime       int         `xml:"runtime,omitempty"` // minutes
	Thumb         []NFOThumb  `xml:"thumb,omitempty"`
	Fanart        *NFOFanart  `xml:"fanart,omitempty"`
	MPAA          string      `xml:"mpaa"`
	ID            string      `xml:"id"`
	UniqueID      NFOUniqueID `xml:"uniqueid"`
	Genre         []string    `xml:"genre,omitempty"`
	Tag           []string    `xml:"tag,omitempty"`
	Studio        string      `xml:"studio,omitempty"`
	Director      string      `xml:"director,omitempty"`
	Premiered     string      `xml:"premiered,omitempty"`
	Year          string      `xml:"year,omitempty"`
	Actor         []NFOActor  `xml:"actor,omitempty"`
}

type NFOThumb struct {
	Aspect string `xml:"aspect,attr,omitempty"`
	URL    string `xml:",chardata"`
}

type NFOFanart struct {
	Thumb []NFOThumb `xml:"thumb"`
}

type NFOUniqueID struct {
	Type    string `xml:"type,attr"`
	Default string `xml:"default,attr"`
	Value   string `xml:",chardata"`
}

type NFOActor struct {
	Name  string `xml:"name"`
	Role  string `xml:"role"`
	Thumb string `xml:"thumb,omitempty"`
}

// GenerateNFOFile produces a standard Kodi/Jellyfin/Plex compatible .nfo file
func GenerateNFOFile(outputDir string, title string, videoID string, channel string, durationSec int64, uploadDate string, description string, categories []string, tags []string, thumbFilename string, bannerFilename string, avatarFilename string) error {
	if outputDir == "" || title == "" {
		return fmt.Errorf("invalid output directory or title")
	}

	runtimeMins := int(durationSec / 60)
	if runtimeMins == 0 && durationSec > 0 {
		runtimeMins = 1
	}

	premiered := ""
	year := ""
	cleanDate := strings.TrimSpace(uploadDate)
	if len(cleanDate) == 8 {
		if t, err := time.Parse("20060102", cleanDate); err == nil {
			premiered = t.Format("2006-01-02")
			year = t.Format("2006")
		}
	} else if len(cleanDate) == 10 && strings.Contains(cleanDate, "-") {
		premiered = cleanDate
		year = cleanDate[:4]
	}

	nfo := NFOMovie{
		Title:         title,
		OriginalTitle: title,
		SortTitle:     title,
		Plot:          description,
		Outline:       getOutline(description),
		Runtime:       runtimeMins,
		MPAA:          "Unrated",
		ID:            videoID,
		UniqueID: NFOUniqueID{
			Type:    "youtube",
			Default: "true",
			Value:   videoID,
		},
		Genre:     categories,
		Tag:       tags,
		Studio:    channel,
		Director:  channel,
		Premiered: premiered,
		Year:      year,
	}

	if thumbFilename != "" {
		nfo.Thumb = append(nfo.Thumb, NFOThumb{Aspect: "poster", URL: thumbFilename})
	}
	if bannerFilename != "" {
		nfo.Fanart = &NFOFanart{
			Thumb: []NFOThumb{{Aspect: "landscape", URL: bannerFilename}},
		}
	}

	if channel != "" {
		nfo.Actor = append(nfo.Actor, NFOActor{
			Name:  channel,
			Role:  "Creator / Host",
			Thumb: avatarFilename,
		})
	}

	xmlData, err := xml.MarshalIndent(nfo, "", "  ")
	if err != nil {
		return err
	}

	xmlHeader := []byte("<?xml version=\"1.0\" encoding=\"UTF-8\" standalone=\"yes\"?>\n")
	fullNFO := append(xmlHeader, xmlData...)

	// Write movie.nfo and <title>.nfo
	_ = os.WriteFile(filepath.Join(outputDir, "movie.nfo"), fullNFO, 0644)
	nfoPath := filepath.Join(outputDir, title+".nfo")
	if err := os.WriteFile(nfoPath, fullNFO, 0644); err != nil {
		return err
	}

	logger.Infof("[NFO] Generated Jellyfin/Plex/Kodi metadata: %s", nfoPath)
	return nil
}

func getOutline(desc string) string {
	if desc == "" {
		return ""
	}
	lines := strings.Split(desc, "\n")
	for _, l := range lines {
		l = strings.TrimSpace(l)
		if l != "" {
			if len(l) > 200 {
				return l[:197] + "..."
			}
			return l
		}
	}
	return ""
}
