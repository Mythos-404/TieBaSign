package main

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type TeiBaUser struct {
	bduss string
	tbs   string

	followedPostings []FollowedPosting
}

type FollowedPosting struct {
	name string
	id   string
	leve string
}

const (
	URL_LIKIE = "http://c.tieba.baidu.com/c/f/forum/like"
	URL_TBS   = "http://tieba.baidu.com/dc/common/tbs"
	URL_SIGN  = "http://c.tieba.baidu.com/c/c/forum/sign"
)

func main() {
	BDUSS := os.Getenv("BDUSS")
	if BDUSS == "" {
		log.Panicln("Error: BDUSS environment variable is not set or is empty")
	}

	var wg sync.WaitGroup
	users := strings.Split(BDUSS, "#")
	for i, user := range users {
		i += 1
		wg.Add(1)
		go func() {
			defer wg.Done()

			log.Printf("Start signing in %vst user", i)

			client := &TeiBaUser{bduss: user}
			client.autoSign()

			log.Printf("Finish signing in %vst user", i)
		}()
	}

	wg.Wait()
}

func (tb *TeiBaUser) autoSign() {
	log.Println("\tGetting tbs started")
	tb.tbs = tb.getTbs()

	log.Println("\tGetting followed postings started")
	tb.followedPostings = tb.getFollowedPostings()

	log.Println("\tStart signing followed postings")
	tb.singFollowedPostings()
}

func (tb *TeiBaUser) getTbs() string {
	type TbsResponse struct {
		Tbs string `json:"tbs"`
	}

	client := &http.Client{}
	req, err := http.NewRequest("GET", URL_TBS, nil)
	if err != nil {
		log.Panicln("Error: Failed to create request:", err)
	}

	req.Header.Set("Cookie", "BDUSS="+tb.bduss)

	resp, err := client.Do(req)
	if err != nil {
		log.Panicln("Error: Get tbs network error:", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Panicln("Error: Failed to read response body:", err)
	}

	var result TbsResponse
	if err := json.Unmarshal(body, &result); err != nil {
		log.Panicln("Error: Cannot unmarshal JSON:", err)
	}
	return result.Tbs
}

func (tb *TeiBaUser) getFollowedPostings() []FollowedPosting {
	type ForumResponse struct {
		ForumList struct {
			NonGconforum []struct {
				ID      string `json:"id"`
				Name    string `json:"name"`
				LevelID string `json:"level_id"`
			} `json:"non-gconforum"`
			Gconforum []struct {
				ID      string `json:"id"`
				Name    string `json:"name"`
				LevelID string `json:"level_id"`
			} `json:"gconforum"`
		} `json:"forum_list"`
		HasMore string `json:"has_more"`
	}

	type RequestData struct {
		ClientVersion string `json:"_client_version"`
		BDUSS         string `json:"BDUSS"`
		PageSize      int    `json:"page_size"`
		PageNo        int    `json:"page_no"`
		Sign          string `json:"sign"`
	}

	fBas := make([]FollowedPosting, 0)
	data := &RequestData{
		ClientVersion: "9.7.8.0",
		BDUSS:         tb.bduss,
		PageSize:      200,

		PageNo: 1,
	}

	for {
		data = encodeData(data)
		formData := url.Values{
			"BDUSS":           {data.BDUSS},
			"_client_version": {data.ClientVersion},
			"page_no":         {strconv.Itoa(data.PageNo)},
			"page_size":       {strconv.Itoa(data.PageSize)},
			"sign":            {data.Sign},
		}

		resp, err := http.PostForm(URL_LIKIE, formData)
		if err != nil {
			log.Panicln("Error: PostForm followed posting error:", err)
		}

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			log.Panicln("Error: Failed to read response body:", err)
		}

		var result ForumResponse
		if err := json.Unmarshal(body, &result); err != nil {
			log.Panicln("Error: Cannot unmarshal JSON:", err)
		}

		for _, forum := range result.ForumList.Gconforum {
			fBas = append(fBas, FollowedPosting{name: forum.Name, id: forum.ID, leve: forum.LevelID})
		}

		for _, forum := range result.ForumList.NonGconforum {
			fBas = append(fBas, FollowedPosting{name: forum.Name, id: forum.ID, leve: forum.LevelID})
		}

		if result.HasMore == "0" {
			break
		}
		data.PageNo += 1
	}

	return fBas
}

func (t *TeiBaUser) singFollowedPostings() {
	var wg sync.WaitGroup
	wg.Add(len(t.followedPostings))
	for _, fp := range t.followedPostings {
		go func() {
			defer wg.Done()

			log.Println("\t    Start signing", strings.Repeat("*", len([]rune(fp.name))))
			fp.sing(t.bduss, t.tbs)
			log.Println("\t    Finish signing", strings.Repeat("*", len([]rune(fp.name))))
		}()
	}

	wg.Wait()
}

func (fp *FollowedPosting) sing(bduss string, tbs string) {
	type RequestData struct {
		ClientVersion string `json:"_client_version"`
		BDUSS         string `json:"BDUSS"`
		TBS           string `json:"tbs"`
		FID           string `json:"fid"`
		KW            string `json:"kw"`
		TIMESTAMP     string `json:"timestamp"`
		Sign          string `json:"sign"`
	}
	data := &RequestData{
		ClientVersion: "9.7.8.0",
		BDUSS:         bduss,
		TBS:           tbs,
		FID:           fp.id,
		KW:            fp.name,

		TIMESTAMP: strconv.FormatInt(time.Now().Unix(), 10),
	}
	data = encodeData(data)

	formData := url.Values{
		"_client_version": {data.ClientVersion},
		"BDUSS":           {data.BDUSS},
		"tbs":             {data.TBS},
		"fid":             {data.FID},
		"kw":              {data.KW},
		"timestamp":       {data.TIMESTAMP},
		"sign":            {data.Sign},
	}

	_, err := http.PostForm(URL_SIGN, formData)
	if err != nil {
		log.Panicln("Error: PostForm followed posting error:", err)
	}
}

func encodeData[T any](data *T) *T {
	v := reflect.ValueOf(data).Elem()
	t := reflect.TypeOf(*data)

	var keys []string
	fieldMap := make(map[string]string)

	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		jsonTag := field.Tag.Get("json")
		if jsonTag == "sign" {
			continue
		}
		value := v.Field(i).Interface()
		fieldMap[jsonTag] = fmt.Sprintf("%v", value)
		keys = append(keys, jsonTag)
	}

	sort.Strings(keys)

	var builder strings.Builder
	for _, key := range keys {
		builder.WriteString(key)
		builder.WriteString("=")
		builder.WriteString(fieldMap[key])
	}

	signString := builder.String() + "tiebaclient!!!"
	hash := md5.Sum([]byte(signString))
	sign := strings.ToUpper(hex.EncodeToString(hash[:]))

	signField := v.FieldByName("Sign")
	if signField.IsValid() && signField.CanSet() {
		signField.SetString(sign)
	}

	return data
}
