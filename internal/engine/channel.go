package engine

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/AvenalJ/yt-archiver/internal/logger"
)

type ChannelLink struct {
	Title string `json:"title"`
	URL   string `json:"url"`
}

type ChannelCommunityPost struct {
	PostID    string `json:"post_id"`
	Published string `json:"published"`
	Text      string `json:"text"`
	ImageURL  string `json:"image_url,omitempty"`
	LikeCount string `json:"like_count,omitempty"`
}

type ChannelPlaylistItem struct {
	PlaylistID   string `json:"playlist_id"`
	Title        string `json:"title"`
	ThumbnailURL string `json:"thumbnail_url"`
	VideoCount   string `json:"video_count"`
	URL          string `json:"url"`
}

type ChannelMetadata struct {
	ChannelID            string                 `json:"channel_id"`
	Title                string                 `json:"title"`
	Handle               string                 `json:"handle"`
	URL                  string                 `json:"url"`
	CanonicalURL         string                 `json:"canonical_url"`
	AvatarURL            string                 `json:"avatar_url"`
	AvatarFilename       string                 `json:"avatar_filename"`
	BannerURL            string                 `json:"banner_url"`
	BannerFilename       string                 `json:"banner_filename"`
	SubscriberCount      int64                  `json:"subscriber_count"`
	FormattedSubscribers string                 `json:"formatted_subscribers"`
	TotalVideos          int                    `json:"total_videos"`
	TotalVideosText      string                 `json:"total_videos_text"`
	TotalViews           int64                  `json:"total_views"`
	FormattedTotalViews  string                 `json:"formatted_total_views"`
	JoinedDate           string                 `json:"joined_date"`
	Country              string                 `json:"country"`
	Description          string                 `json:"description"`
	IsVerified           bool                   `json:"is_verified"`
	Links                []ChannelLink          `json:"links,omitempty"`
	Community            []ChannelCommunityPost `json:"community,omitempty"`
	Posts                []ChannelCommunityPost `json:"posts,omitempty"`
	Playlists            []ChannelPlaylistItem  `json:"playlists,omitempty"`
}

var (
	ytInitialDataRegex = regexp.MustCompile(`var ytInitialData\s*=\s*({.+?});</script>`)
	apiKeyRegex        = regexp.MustCompile(`"(?:INNERTUBE_API_KEY|innertubeApiKey)":\s*"([^"]+)"`)
)

// FetchChannelMetadata fetches rich channel information, downloads banner and avatar, and writes channel.json
func FetchChannelMetadata(ctx context.Context, channelURL string, channelTitle string, outputDir string) *ChannelMetadata {
	meta := &ChannelMetadata{
		Title:          channelTitle,
		URL:            channelURL,
		AvatarFilename: "channel_avatar.jpg",
		BannerFilename: "channel_banner.jpg",
		Links:          []ChannelLink{},
		Community:      []ChannelCommunityPost{},
		Posts:          []ChannelCommunityPost{},
		Playlists:      []ChannelPlaylistItem{},
	}

	if channelURL == "" {
		return meta
	}

	// Try reading existing channel.json if present in outputDir
	if outputDir != "" {
		channelJSONPath := filepath.Join(outputDir, "channel.json")
		if data, err := os.ReadFile(channelJSONPath); err == nil {
			var existing ChannelMetadata
			if json.Unmarshal(data, &existing) == nil && existing.Title != "" && existing.FormattedSubscribers != "" {
				if _, err := os.Stat(filepath.Join(outputDir, existing.AvatarFilename)); err == nil {
					if _, err := os.Stat(filepath.Join(outputDir, existing.BannerFilename)); err == nil {
						return &existing
					}
				}
			}
		}
	}

	var apiKey string

	// Fetch /about endpoint
	aboutURL := strings.TrimRight(channelURL, "/")
	if !strings.HasSuffix(aboutURL, "/about") {
		aboutURL += "/about"
	}

	body := fetchChannelHTML(ctx, aboutURL)
	if body != "" {
		if km := apiKeyRegex.FindStringSubmatch(body); len(km) > 1 {
			apiKey = km[1]
		}
		if m := ytInitialDataRegex.FindStringSubmatch(body); len(m) > 1 {
			parseInitialData(m[1], meta)
		}
	}

	// Fetch /playlists tab
	playlistsURL := strings.TrimRight(channelURL, "/about") + "/playlists"
	if pBody := fetchChannelHTML(ctx, playlistsURL); pBody != "" {
		if apiKey == "" {
			if km := apiKeyRegex.FindStringSubmatch(pBody); len(km) > 1 {
				apiKey = km[1]
			}
		}
		if m := ytInitialDataRegex.FindStringSubmatch(pBody); len(m) > 1 {
			parsePlaylistsData(ctx, m[1], apiKey, meta)
		}
	}

	// Fetch /posts or /community tab
	postsURL := strings.TrimRight(channelURL, "/about") + "/posts"
	if cBody := fetchChannelHTML(ctx, postsURL); cBody != "" {
		if apiKey == "" {
			if km := apiKeyRegex.FindStringSubmatch(cBody); len(km) > 1 {
				apiKey = km[1]
			}
		}
		if m := ytInitialDataRegex.FindStringSubmatch(cBody); len(m) > 1 {
			parseCommunityData(ctx, m[1], apiKey, meta)
		}
	}

	meta.Posts = meta.Community

	// Fallback handle from URL if missing
	if meta.Handle == "" && strings.Contains(channelURL, "/@") {
		parts := strings.Split(channelURL, "/@")
		if len(parts) > 1 {
			meta.Handle = "@" + strings.Split(parts[1], "/")[0]
		}
	}

	if meta.FormattedSubscribers != "" && meta.SubscriberCount == 0 {
		meta.SubscriberCount = parseSubCount(meta.FormattedSubscribers)
	}

	// If output directory is specified, download assets and save metadata files
	if outputDir != "" {
		_ = os.MkdirAll(outputDir, 0755)

		// Download Avatar Image
		avatarDest := filepath.Join(outputDir, meta.AvatarFilename)
		if meta.AvatarURL != "" {
			downloadFile(ctx, meta.AvatarURL, avatarDest)
		} else {
			meta.AvatarURL = extractChannelAvatarURL(ctx, channelURL)
			if meta.AvatarURL != "" {
				downloadFile(ctx, meta.AvatarURL, avatarDest)
			}
		}

		// Download Banner Image
		bannerDest := filepath.Join(outputDir, meta.BannerFilename)
		if meta.BannerURL != "" {
			downloadFile(ctx, meta.BannerURL, bannerDest)
		}

		// Save structured channel.json
		channelJSONPath := filepath.Join(outputDir, "channel.json")
		if jsonBytes, err := json.MarshalIndent(meta, "", "  "); err == nil {
			_ = os.WriteFile(channelJSONPath, jsonBytes, 0644)
		}

		if len(meta.Community) > 0 {
			if commBytes, err := json.MarshalIndent(meta.Community, "", "  "); err == nil {
				_ = os.WriteFile(filepath.Join(outputDir, "community.json"), commBytes, 0644)
				_ = os.WriteFile(filepath.Join(outputDir, "posts.json"), commBytes, 0644)
			}
		}

		if len(meta.Playlists) > 0 {
			if playBytes, err := json.MarshalIndent(meta.Playlists, "", "  "); err == nil {
				_ = os.WriteFile(filepath.Join(outputDir, "playlists.json"), playBytes, 0644)
			}
		}
	}

	logger.Infof("[Channel] Archived profile for channel: %s (%s, %s subs, %d playlists, %d posts)",
		meta.Title, meta.Handle, meta.FormattedSubscribers, len(meta.Playlists), len(meta.Community))
	return meta
}

func fetchChannelHTML(ctx context.Context, targetURL string) string {
	client := &http.Client{Timeout: 15 * time.Second}
	req, err := http.NewRequestWithContext(ctx, "GET", targetURL, nil)
	if err != nil {
		return ""
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")

	resp, err := client.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		return ""
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(io.LimitReader(resp.Body, 5*1024*1024))
	if err != nil {
		return ""
	}
	return string(bodyBytes)
}

func fetchInnerTubeBrowse(ctx context.Context, apiKey string, continuationToken string) []byte {
	if apiKey == "" || continuationToken == "" {
		return nil
	}
	endpoint := fmt.Sprintf("https://www.youtube.com/youtubei/v1/browse?key=%s", apiKey)
	payload := map[string]interface{}{
		"context": map[string]interface{}{
			"client": map[string]interface{}{
				"clientName":    "WEB",
				"clientVersion": "2.20240501.01.00",
				"hl":            "en",
				"gl":            "US",
			},
		},
		"continuation": continuationToken,
	}
	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return nil
	}

	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		return nil
	}
	defer resp.Body.Close()

	resBytes, err := io.ReadAll(io.LimitReader(resp.Body, 5*1024*1024))
	if err != nil {
		return nil
	}
	return resBytes
}

func parseInitialData(rawJSON string, meta *ChannelMetadata) {
	var root map[string]interface{}
	if err := json.Unmarshal([]byte(rawJSON), &root); err != nil {
		return
	}

	// 1. Parse Header (pageHeaderRenderer)
	if header, ok := root["header"].(map[string]interface{}); ok {
		if phr, ok := header["pageHeaderRenderer"].(map[string]interface{}); ok {
			if content, ok := phr["content"].(map[string]interface{}); ok {
				if phvm, ok := content["pageHeaderViewModel"].(map[string]interface{}); ok {
					// Title
					if titleObj, ok := phvm["title"].(map[string]interface{}); ok {
						if dtvm, ok := titleObj["dynamicTextViewModel"].(map[string]interface{}); ok {
							if textObj, ok := dtvm["text"].(map[string]interface{}); ok {
								if t, ok := textObj["content"].(string); ok && t != "" {
									meta.Title = t
								}
							}
						}
					}

					// Banner
					if bannerObj, ok := phvm["banner"].(map[string]interface{}); ok {
						if ibvm, ok := bannerObj["imageBannerViewModel"].(map[string]interface{}); ok {
							if img, ok := ibvm["image"].(map[string]interface{}); ok {
								if sources, ok := img["sources"].([]interface{}); ok && len(sources) > 0 {
									bestSource := sources[len(sources)-1].(map[string]interface{})
									if u, ok := bestSource["url"].(string); ok {
										meta.BannerURL = u
									}
								}
							}
						}
					}

					// Avatar
					if imgObj, ok := phvm["image"].(map[string]interface{}); ok {
						if davm, ok := imgObj["decoratedAvatarViewModel"].(map[string]interface{}); ok {
							if avObj, ok := davm["avatar"].(map[string]interface{}); ok {
								if avm, ok := avObj["avatarViewModel"].(map[string]interface{}); ok {
									if img, ok := avm["image"].(map[string]interface{}); ok {
										if sources, ok := img["sources"].([]interface{}); ok && len(sources) > 0 {
											bestSource := sources[len(sources)-1].(map[string]interface{})
											if u, ok := bestSource["url"].(string); ok {
												meta.AvatarURL = u
											}
										}
									}
								}
							}
						}
					}

					// Metadata Rows (Handle, Subscribers, Videos)
					if metaObj, ok := phvm["metadata"].(map[string]interface{}); ok {
						if cmvm, ok := metaObj["contentMetadataViewModel"].(map[string]interface{}); ok {
							if rows, ok := cmvm["metadataRows"].([]interface{}); ok {
								for _, r := range rows {
									if rMap, ok := r.(map[string]interface{}); ok {
										if parts, ok := rMap["metadataParts"].([]interface{}); ok {
											for _, p := range parts {
												if pMap, ok := p.(map[string]interface{}); ok {
													if textObj, ok := pMap["text"].(map[string]interface{}); ok {
														if txt, ok := textObj["content"].(string); ok {
															txt = strings.TrimSpace(txt)
															if strings.HasPrefix(txt, "@") {
																meta.Handle = txt
															} else if strings.Contains(strings.ToLower(txt), "subscriber") {
																meta.FormattedSubscribers = txt
																meta.SubscriberCount = parseSubCount(txt)
															} else if strings.Contains(strings.ToLower(txt), "video") {
																meta.TotalVideosText = txt
																meta.TotalVideos = int(parseSubCount(txt))
															}
														}
													}
												}
											}
										}
									}
								}
							}
						}
					}
				}
			}
		}
	}

	// 2. Parse About Channel Renderer (Engagement Panel)
	if eps, ok := root["onResponseReceivedEndpoints"].([]interface{}); ok {
		for _, ep := range eps {
			if epMap, ok := ep.(map[string]interface{}); ok {
				if spe, ok := epMap["showEngagementPanelEndpoint"].(map[string]interface{}); ok {
					if panel, ok := spe["engagementPanel"].(map[string]interface{}); ok {
						if epslr, ok := panel["engagementPanelSectionListRenderer"].(map[string]interface{}); ok {
							if cnt, ok := epslr["content"].(map[string]interface{}); ok {
								if slr, ok := cnt["sectionListRenderer"].(map[string]interface{}); ok {
									if contents, ok := slr["contents"].([]interface{}); ok && len(contents) > 0 {
										if c0, ok := contents[0].(map[string]interface{}); ok {
											if isr, ok := c0["itemSectionRenderer"].(map[string]interface{}); ok {
												if isrContents, ok := isr["contents"].([]interface{}); ok && len(isrContents) > 0 {
													if ic0, ok := isrContents[0].(map[string]interface{}); ok {
														if acr, ok := ic0["aboutChannelRenderer"].(map[string]interface{}); ok {
															extractAboutData(acr, meta)
														}
													}
												}
											}
										}
									}
								}
							}
						}
					}
				}
			}
		}
	}

	// Check verified badge
	rawStr := rawJSON
	if strings.Contains(rawStr, "CHECK_CIRCLE_FILLED") || strings.Contains(rawStr, "BADGE_STYLE_TYPE_VERIFIED") {
		meta.IsVerified = true
	}
}

func parsePlaylistItemMap(itMap map[string]interface{}) *ChannelPlaylistItem {
	if lvm, ok := itMap["lockupViewModel"].(map[string]interface{}); ok {
		pTitle := ""
		if tObj, ok := lvm["metadata"].(map[string]interface{})["lockupMetadataViewModel"].(map[string]interface{})["title"].(map[string]interface{}); ok {
			pTitle, _ = tObj["content"].(string)
		}
		count := ""
		if md, ok := lvm["metadata"].(map[string]interface{})["lockupMetadataViewModel"].(map[string]interface{})["metadata"].(map[string]interface{})["contentMetadataViewModel"].(map[string]interface{})["metadataRows"].([]interface{}); ok && len(md) > 0 {
			if parts, ok := md[0].(map[string]interface{})["metadataParts"].([]interface{}); ok && len(parts) > 0 {
				count, _ = parts[0].(map[string]interface{})["text"].(map[string]interface{})["content"].(string)
			}
		}
		thumb := ""
		if pThumb, ok := lvm["contentImage"].(map[string]interface{})["collectionThumbnailViewModel"].(map[string]interface{})["primaryThumbnail"].(map[string]interface{})["thumbnailViewModel"].(map[string]interface{})["image"].(map[string]interface{})["sources"].([]interface{}); ok && len(pThumb) > 0 {
			thumb, _ = pThumb[len(pThumb)-1].(map[string]interface{})["url"].(string)
		}
		cID, _ := lvm["contentId"].(string)

		if pTitle != "" {
			return &ChannelPlaylistItem{
				PlaylistID:   cID,
				Title:        pTitle,
				ThumbnailURL: thumb,
				VideoCount:   count,
				URL:          "https://www.youtube.com/playlist?list=" + cID,
			}
		}
	} else if gpr, ok := itMap["gridPlaylistRenderer"].(map[string]interface{}); ok {
		pTitle := ""
		if tObj, ok := gpr["title"].(map[string]interface{})["runs"].([]interface{}); ok && len(tObj) > 0 {
			pTitle, _ = tObj[0].(map[string]interface{})["text"].(string)
		}
		count := ""
		if cObj, ok := gpr["videoCountShortText"].(map[string]interface{}); ok {
			count, _ = cObj["simpleText"].(string)
		}
		pID, _ := gpr["playlistId"].(string)
		thumb := ""
		if tArr, ok := gpr["thumbnail"].(map[string]interface{})["thumbnails"].([]interface{}); ok && len(tArr) > 0 {
			thumb, _ = tArr[len(tArr)-1].(map[string]interface{})["url"].(string)
		}
		if pTitle != "" {
			return &ChannelPlaylistItem{
				PlaylistID:   pID,
				Title:        pTitle,
				ThumbnailURL: thumb,
				VideoCount:   count,
				URL:          "https://www.youtube.com/playlist?list=" + pID,
			}
		}
	}
	return nil
}

func parsePlaylistsData(ctx context.Context, rawJSON string, apiKey string, meta *ChannelMetadata) {
	var root map[string]interface{}
	if err := json.Unmarshal([]byte(rawJSON), &root); err != nil {
		return
	}

	tabs, ok := root["contents"].(map[string]interface{})["twoColumnBrowseResultsRenderer"].(map[string]interface{})["tabs"].([]interface{})
	if !ok {
		return
	}

	continuationToken := ""

	for _, t := range tabs {
		tr, ok := t.(map[string]interface{})["tabRenderer"].(map[string]interface{})
		if !ok {
			continue
		}
		if title, _ := tr["title"].(string); title == "Playlists" {
			contents, _ := tr["content"].(map[string]interface{})["sectionListRenderer"].(map[string]interface{})["contents"].([]interface{})
			for _, c := range contents {
				itemSec, _ := c.(map[string]interface{})["itemSectionRenderer"].(map[string]interface{})
				items, _ := itemSec["contents"].([]interface{})[0].(map[string]interface{})["gridRenderer"].(map[string]interface{})["items"].([]interface{})
				if len(items) == 0 {
					items, _ = itemSec["contents"].([]interface{})
				}

				for _, it := range items {
					itMap, ok := it.(map[string]interface{})
					if !ok {
						continue
					}
					if pl := parsePlaylistItemMap(itMap); pl != nil {
						meta.Playlists = append(meta.Playlists, *pl)
					} else if contItem, ok := itMap["continuationItemRenderer"].(map[string]interface{}); ok {
						if ce, ok := contItem["continuationEndpoint"].(map[string]interface{})["continuationCommand"].(map[string]interface{}); ok {
							continuationToken, _ = ce["token"].(string)
						}
					}
				}
			}
		}
	}

	// Deep pagination loop for playlists (up to 150 playlists)
	for page := 0; page < 10 && continuationToken != "" && len(meta.Playlists) < 150; page++ {
		resBytes := fetchInnerTubeBrowse(ctx, apiKey, continuationToken)
		if len(resBytes) == 0 {
			break
		}
		continuationToken = ""
		var resRoot map[string]interface{}
		if err := json.Unmarshal(resBytes, &resRoot); err != nil {
			break
		}
		if eps, ok := resRoot["onResponseReceivedEndpoints"].([]interface{}); ok {
			for _, ep := range eps {
				if epMap, ok := ep.(map[string]interface{}); ok {
					if act, ok := epMap["appendContinuationItemsAction"].(map[string]interface{}); ok {
						if contItems, ok := act["continuationItems"].([]interface{}); ok {
							for _, it := range contItems {
								if itMap, ok := it.(map[string]interface{}); ok {
									if pl := parsePlaylistItemMap(itMap); pl != nil {
										meta.Playlists = append(meta.Playlists, *pl)
									} else if contItem, ok := itMap["continuationItemRenderer"].(map[string]interface{}); ok {
										if ce, ok := contItem["continuationEndpoint"].(map[string]interface{})["continuationCommand"].(map[string]interface{}); ok {
											continuationToken, _ = ce["token"].(string)
										}
									}
								}
							}
						}
					}
				}
			}
		}
	}
}

func parsePostItemMap(itMap map[string]interface{}) *ChannelCommunityPost {
	bpr := itMap["backstagePostRenderer"]
	if bpr == nil {
		if thread, ok := itMap["backstagePostThreadRenderer"].(map[string]interface{}); ok {
			bpr = thread["post"].(map[string]interface{})["backstagePostRenderer"]
		}
	}
	if bprMap, ok := bpr.(map[string]interface{}); ok {
		postID, _ := bprMap["postId"].(string)
		var sb strings.Builder
		if ct, ok := bprMap["contentText"].(map[string]interface{})["runs"].([]interface{}); ok {
			for _, r := range ct {
				if rMap, ok := r.(map[string]interface{}); ok {
					txt, _ := rMap["text"].(string)
					sb.WriteString(txt)
				}
			}
		}
		pubDate := ""
		if pt, ok := bprMap["publishedTimeText"].(map[string]interface{})["runs"].([]interface{}); ok && len(pt) > 0 {
			pubDate, _ = pt[0].(map[string]interface{})["text"].(string)
		}
		likes := ""
		if vc, ok := bprMap["voteCount"].(map[string]interface{}); ok {
			likes, _ = vc["simpleText"].(string)
		}
		imgURL := ""
		if att, ok := bprMap["backstageAttachment"].(map[string]interface{}); ok {
			if bir, ok := att["backstageImageRenderer"].(map[string]interface{}); ok {
				if thumbs, ok := bir["image"].(map[string]interface{})["thumbnails"].([]interface{}); ok && len(thumbs) > 0 {
					imgURL, _ = thumbs[len(thumbs)-1].(map[string]interface{})["url"].(string)
				}
			}
		}

		return &ChannelCommunityPost{
			PostID:    postID,
			Published: pubDate,
			Text:      sb.String(),
			ImageURL:  imgURL,
			LikeCount: likes,
		}
	}
	return nil
}

func parseCommunityData(ctx context.Context, rawJSON string, apiKey string, meta *ChannelMetadata) {
	var root map[string]interface{}
	if err := json.Unmarshal([]byte(rawJSON), &root); err != nil {
		return
	}

	tabs, ok := root["contents"].(map[string]interface{})["twoColumnBrowseResultsRenderer"].(map[string]interface{})["tabs"].([]interface{})
	if !ok {
		return
	}

	continuationToken := ""

	for _, t := range tabs {
		tr, ok := t.(map[string]interface{})["tabRenderer"].(map[string]interface{})
		if !ok {
			continue
		}
		title, _ := tr["title"].(string)
		if title == "Posts" || title == "Community" {
			contents, _ := tr["content"].(map[string]interface{})["sectionListRenderer"].(map[string]interface{})["contents"].([]interface{})
			for _, c := range contents {
				itemSec, _ := c.(map[string]interface{})["itemSectionRenderer"].(map[string]interface{})
				items, _ := itemSec["contents"].([]interface{})
				for _, it := range items {
					itMap, ok := it.(map[string]interface{})
					if !ok {
						continue
					}
					if post := parsePostItemMap(itMap); post != nil {
						meta.Community = append(meta.Community, *post)
					} else if contItem, ok := itMap["continuationItemRenderer"].(map[string]interface{}); ok {
						if ce, ok := contItem["continuationEndpoint"].(map[string]interface{})["continuationCommand"].(map[string]interface{}); ok {
							continuationToken, _ = ce["token"].(string)
						}
					}
				}
			}
		}
	}

	// Deep pagination loop for Posts (up to 300 posts across up to 30 continuation pages)
	for page := 0; page < 30 && continuationToken != "" && len(meta.Community) < 300; page++ {
		resBytes := fetchInnerTubeBrowse(ctx, apiKey, continuationToken)
		if len(resBytes) == 0 {
			break
		}
		continuationToken = ""
		var resRoot map[string]interface{}
		if err := json.Unmarshal(resBytes, &resRoot); err != nil {
			break
		}
		if eps, ok := resRoot["onResponseReceivedEndpoints"].([]interface{}); ok {
			for _, ep := range eps {
				if epMap, ok := ep.(map[string]interface{}); ok {
					if act, ok := epMap["appendContinuationItemsAction"].(map[string]interface{}); ok {
						if contItems, ok := act["continuationItems"].([]interface{}); ok {
							for _, it := range contItems {
								if itMap, ok := it.(map[string]interface{}); ok {
									if post := parsePostItemMap(itMap); post != nil {
										meta.Community = append(meta.Community, *post)
									} else if contItem, ok := itMap["continuationItemRenderer"].(map[string]interface{}); ok {
										if ce, ok := contItem["continuationEndpoint"].(map[string]interface{})["continuationCommand"].(map[string]interface{}); ok {
											continuationToken, _ = ce["token"].(string)
										}
									}
								}
							}
						}
					}
				}
			}
		}
	}
}

func extractAboutData(acr map[string]interface{}, meta *ChannelMetadata) {
	metaObj, ok := acr["metadata"].(map[string]interface{})
	if !ok {
		return
	}
	vm, ok := metaObj["aboutChannelViewModel"].(map[string]interface{})
	if !ok {
		return
	}

	if desc, ok := vm["description"].(string); ok && desc != "" {
		meta.Description = desc
	}
	if country, ok := vm["country"].(string); ok && country != "" {
		meta.Country = country
	}
	if joinedObj, ok := vm["joinedDateText"].(map[string]interface{}); ok {
		if jText, ok := joinedObj["content"].(string); ok && jText != "" {
			meta.JoinedDate = jText
		}
	}
	if subs, ok := vm["subscriberCountText"].(string); ok && subs != "" {
		meta.FormattedSubscribers = subs
		meta.SubscriberCount = parseSubCount(subs)
	}
	if vids, ok := vm["videoCountText"].(string); ok && vids != "" {
		meta.TotalVideosText = vids
		cleanCount := strings.ReplaceAll(strings.Split(vids, " ")[0], ",", "")
		if count, err := strconv.Atoi(cleanCount); err == nil {
			meta.TotalVideos = count
		}
	}
	if views, ok := vm["viewCountText"].(string); ok && views != "" {
		meta.FormattedTotalViews = views
		cleanViews := strings.ReplaceAll(strings.Split(views, " ")[0], ",", "")
		if v, err := strconv.ParseInt(cleanViews, 10, 64); err == nil {
			meta.TotalViews = v
		}
	}
	if canon, ok := vm["canonicalChannelUrl"].(string); ok && canon != "" {
		meta.CanonicalURL = canon
	}

	// Links
	if linksArr, ok := vm["links"].([]interface{}); ok {
		meta.Links = []ChannelLink{}
		for _, l := range linksArr {
			if lMap, ok := l.(map[string]interface{}); ok {
				if ext, ok := lMap["channelExternalLinkViewModel"].(map[string]interface{}); ok {
					title := ""
					if tObj, ok := ext["title"].(map[string]interface{}); ok {
						if t, ok := tObj["content"].(string); ok {
							title = t
						}
					}
					url := ""
					if lObj, ok := ext["link"].(map[string]interface{}); ok {
						if u, ok := lObj["content"].(string); ok {
							url = u
						}
					}
					if title != "" || url != "" {
						meta.Links = append(meta.Links, ChannelLink{
							Title: title,
							URL:   url,
						})
					}
				}
			}
		}
	}
}

func parseSubCount(s string) int64 {
	s = strings.ToLower(s)
	s = strings.ReplaceAll(s, "subscribers", "")
	s = strings.ReplaceAll(s, "subscriber", "")
	s = strings.TrimSpace(s)

	multiplier := 1.0
	if strings.HasSuffix(s, "m") {
		multiplier = 1000000.0
		s = strings.TrimSuffix(s, "m")
	} else if strings.HasSuffix(s, "k") {
		multiplier = 1000.0
		s = strings.TrimSuffix(s, "k")
	} else if strings.HasSuffix(s, "b") {
		multiplier = 1000000000.0
		s = strings.TrimSuffix(s, "b")
	}

	s = strings.ReplaceAll(s, ",", "")
	s = strings.TrimSpace(s)
	val, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return int64(val * multiplier)
}
