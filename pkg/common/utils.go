package common

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/PuerkitoBio/goquery"
	"html/template"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
)

const (
	DataDragonUrl = "https://ddragon.leagueoflegends.com"
	BaseBootId    = `1001`
)

func MatchSpellName(src string) string {
	if len(src) == 0 {
		return ""
	}

	r := regexp.MustCompile("Summoner(.*)\\.png")
	result := r.FindStringSubmatch(src)
	s := strings.ToLower(result[len(result)-1])
	return s
}

func MatchId(src string) string {
	if len(src) == 0 {
		return ""
	}

	r := regexp.MustCompile("\\/(\\d+)\\.png")
	result := r.FindStringSubmatch(src)
	s := strings.ToLower(result[len(result)-1])
	return s
}

func NoRepeatPush(el string, arr []string) []string {
	for _, v := range arr {
		if v == el {
			return arr
		}
	}

	return append(arr, el)
}

func Includes(target string, list []string) bool {
	existed := false
	for _, i := range list {
		if i == target {
			existed = true
			break
		}
	}

	return existed
}

func MakeRequest(url string) ([]byte, error) {
	res, err := http.Get(url)
	if err != nil {
		return nil, err
	}

	defer res.Body.Close()
	if res.StatusCode != 200 {
		return nil, errors.New(res.Status)
	}

	body, _ := ioutil.ReadAll(res.Body)
	return body, nil
}

func GetChampionList() (*ChampionListResp, string, error) {
	body, err := MakeRequest(DataDragonUrl + "/api/versions.json")
	if err != nil {
		return nil, "", err
	}

	var versionArr []string
	_ = json.Unmarshal(body, &versionArr)
	version := versionArr[0]

	cBody, cErr := MakeRequest(DataDragonUrl + "/cdn/" + version + "/data/en_US/champion.json")
	if cErr != nil {
		return nil, "", errors.New(`data dragon: request champion list failed`)
	}

	var resp ChampionListResp
	_ = json.Unmarshal(cBody, &resp)

	fmt.Printf("🤖 Got official champion list, total %d \n", len(resp.Data))
	return &resp, version, nil
}

func SaveJSON(fileName string, data interface{}) error {
	file, _ := json.MarshalIndent(data, "", "  ")
	wErr := ioutil.WriteFile(fileName, file, 0644)

	if wErr != nil {
		return wErr
	}

	return nil
}

func ParseHTML(url string) (*goquery.Document, error) {
	body, err := MakeRequest(url)
	if err != nil {
		return nil, err
	}

	buf := bytes.NewBuffer(body)
	reader := ioutil.NopCloser(buf)
	return goquery.NewDocumentFromReader(reader)
}

func GenPkgInfo(tplPath string, vars interface{}) (string, error) {
	tpl, err := template.ParseFiles(tplPath)
	if err != nil {
		return "", err
	}

	var tplBytes bytes.Buffer
	err = tpl.Execute(&tplBytes, vars)
	if err != nil {
		return "", err
	}

	return tplBytes.String(), nil
}

func GetItemList(version string) (*map[string]BuildItem, error) {
	body, err := MakeRequest(DataDragonUrl + `/cdn/` + version + `/data/en_US/item.json`)
	if err != nil {
		return nil, err
	}

	var resp BuildItemResp
	_ = json.Unmarshal(body, &resp)
	return &resp.Data, nil
}

func IsBoot(id string, items map[string]BuildItem) bool {
	result := Includes(BaseBootId, items[id].From)
	return result
}

func MakeBuildBlock(arr []string, name string) ItemBuildBlockItem {
	block := ItemBuildBlockItem{
		Type: name,
	}

	for _, id := range arr {
		block.Items = append(block.Items, BlockItem{
			Id:    id,
			Count: 1,
		})
	}

	return block
}

func GetRunesReforged(version string) (IRuneLookUp, IAllRunes, error) {
	body, err := MakeRequest(DataDragonUrl + `/cdn/` + version + `/data/en_US/runesReforged.json`)
	if err != nil {
		return nil, nil, err
	}

	var resp []RuneSlot
	_ = json.Unmarshal(body, &resp)

	//data := make(map[int]RespRuneItem)

	data := make(map[int]*RespRuneItem)
	for _, slot := range resp {
		for j, s := range slot.Slots {
			for _, r := range s.Runes {
				r.Style = slot.Id
				r.Slot = j
				r.Primary = j == 0
				data[r.Id] = &r
			}
		}
	}
	return data, &resp, nil
}

func GetKeys(v interface{}) []string {
	var keys []string
	value := reflect.ValueOf(v)
	if value.Kind() == reflect.Map {
		for _, v := range value.MapKeys() {
			if v.Kind() == reflect.String {
				keys = append(keys, v.String())
			}
		}
	}

	return keys
}

func GetPrimaryIdForRune(id int, runeLookUp IRuneLookUp) int {
	return runeLookUp[id].Style
}

func Write2Folder(result [][]ChampionDataItem, pkgName string, timestamp int64, sourceVersion string, officialVer string) {
	outputPath := filepath.Join(".", "output", pkgName)
	_ = os.MkdirAll(outputPath, os.ModePerm)

	for _, data := range result {
		fileName := outputPath + "/" + data[0].Alias + ".json"
		_ = SaveJSON(fileName, data)
	}

	pkg, _ := GenPkgInfo("tpl/package.json", PkgInfo{
		Timestamp:       timestamp,
		SourceVersion:   sourceVersion,
		OfficialVersion: officialVer,
		PkgName:         pkgName,
	})
	_ = ioutil.WriteFile("output/"+pkgName+"/package.json", []byte(pkg), 0644)
}
