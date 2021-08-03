package lolalytics

import (
	"data-crawler/pkg/common"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

var cidReg = regexp.MustCompile("&cid=\\d+?&")
var laneReg = regexp.MustCompile("&lane=[a-zA-Z]+?&")
var epReg = regexp.MustCompile("ep=.*?region=all")
var tierReg = regexp.MustCompile("&tier=[a-zA-Z_]+&")
var patchReg = regexp.MustCompile("&patch=((\\d+\\.)+\\d+?)&")

const ApiUrl = "https://apix1.op.lol"
const MinimumPickRate = 5

func makeQuery(query string) func(string, string, string) string {
	oldQ := query
	return func(cid string, lane string, tier string) string {
		q := cidReg.ReplaceAllString(oldQ, "&cid="+cid+"&")
		q = laneReg.ReplaceAllString(q, "&lane="+lane+"&")
		q = tierReg.ReplaceAllString(q, "&tier="+tier+"&")
		return q
	}
}

func getSourceVersion(q string) string {
	m := patchReg.FindAllStringSubmatch(q, 1)
	return m[0][1]
}

func getPatchVersion(v string) string {
	strArr := strings.Split(v, ".")
	length := len(strArr)
	versionArr := strArr[:length-1]
	return strings.Join(versionArr, ".")
}

func getTierList(q string) (ITierList, error) {
	var data ITierList

	// list sort by name
	body, err := common.MakeRequest(ApiUrl + "/tierlist/7/?" + q)
	if err != nil {
		return data, err
	}

	_ = json.Unmarshal(body, &data)
	return data, nil
}

func getChampionById(id string, championAliasList map[string]common.ChampionItem) common.ChampionItem {
	var ret common.ChampionItem
	for _, champ := range championAliasList {
		if id != champ.Key {
			continue
		}

		ret = champ
		break
	}

	return ret
}

func makeBlock(title string, set []int) common.ItemBuildBlockItem {
	blockItem := common.ItemBuildBlockItem{
		Type: title,
	}

	for _, itemId := range set {
		item := common.BlockItem{
			Id:    strconv.Itoa(itemId),
			Count: 1,
		}
		blockItem.Items = append(blockItem.Items, item)
	}

	return blockItem
}

func extractItemIds(items []IItemN) []int {
	var ids []int
	for _, i := range items {
		ids = append(ids, i.ID)
	}

	return ids
}

func makeBuildBlocksFromSet(data IItems) []common.ItemBuildBlockItem {
	var blocks []common.ItemBuildBlockItem
	startingTitle := "Starting items, win rate " + fmt.Sprintf("%.2f%%", data.Start.Wr)
	startingBlock := makeBlock(startingTitle, data.Start.Set)
	blocks = append(blocks, startingBlock)

	coreTitle := "Core items, win rate " + fmt.Sprintf("%.2f%%", data.Core.Wr)
	coreBlock := makeBlock(coreTitle, data.Core.Set)
	blocks = append(blocks, coreBlock)

	item4Ids := extractItemIds(data.Item4)
	item4Block := makeBlock("Item 4", item4Ids)
	blocks = append(blocks, item4Block)

	item5Ids := extractItemIds(data.Item5)
	item5Block := makeBlock("Item 5", item5Ids)
	blocks = append(blocks, item5Block)

	item6Ids := extractItemIds(data.Item6)
	item6Block := makeBlock("Item 6", item6Ids)
	blocks = append(blocks, item6Block)

	return blocks
}

func concatRuneIds(pri []int, sec []int, mod []int) []int {
	var ids []int
	ids = append(ids, pri...)
	ids = append(ids, sec...)
	ids = append(ids, mod...)
	return ids
}

func makeBuild(champion common.ChampionItem, query string, sourceVersion string, officialVer string, timestamp int64, cnt int, fetchMore bool, runeLookUp common.IRuneLookUp, aram bool) (*[]common.ChampionDataItem, error) {
	body, err := common.MakeRequest(ApiUrl + "/mega?" + query)

	if err != nil {
		fmt.Println("[lolalytics] Fetch champion data failed.", champion.Id)
		return nil, err
	}

	var resp IChampionData
	_ = json.Unmarshal(body, &resp)
	ID, _ := strconv.Atoi(champion.Key)
	curLane := resp.Header.Lane

	if resp.Summary.Sums == nil {
		errMsg := "[lolalytics] Champion data not ready, " + champion.Name + " " + curLane
		if aram {
			errMsg = "[lolalytics-ARAM] Champion data not ready, " + champion.Name + " " + curLane
		}
		fmt.Println(errMsg)
		return nil, errors.New(errMsg)
	}

	var builds []common.ChampionDataItem
	defaultBuild := common.ChampionDataItem{
		Position:        curLane,
		Index:           cnt,
		Id:              champion.Key,
		Version:         sourceVersion,
		Timestamp:       timestamp,
		Alias:           champion.Id,
		Name:            champion.Name,
		OfficialVersion: officialVer,
	}

	buildTitlePrefix := "[lolalytics]"
	buildTitleSuffix := "@" + curLane + ", " + sourceVersion + " (G+)"
	associatedMaps := []int{11, 12}
	if aram {
		buildTitlePrefix = "[lolalytics-ARAM]"
		buildTitleSuffix = ", " + sourceVersion + " (G+)"
		associatedMaps = []int{12}
	}
	highestWinBuild := common.ItemBuild{
		Title:               buildTitlePrefix + " Highest Win" + buildTitleSuffix,
		AssociatedMaps:      associatedMaps,
		AssociatedChampions: []int{ID},
		Map:                 "any",
		Mode:                "any",
		PreferredItemSlots:  []string{},
		Sortrank:            1,
		StartedFrom:         "blank",
		Type:                "custom",
		Blocks:              makeBuildBlocksFromSet(resp.Summary.Items.Win),
	}
	defaultBuild.ItemBuilds = append(defaultBuild.ItemBuilds, highestWinBuild)
	mostCommonBuild := common.ItemBuild{
		Title:               buildTitlePrefix + " Most Common" + buildTitleSuffix,
		AssociatedMaps:      associatedMaps,
		AssociatedChampions: []int{ID},
		Map:                 "any",
		Mode:                "any",
		PreferredItemSlots:  []string{},
		Sortrank:            1,
		StartedFrom:         "blank",
		Type:                "custom",
		Blocks:              makeBuildBlocksFromSet(resp.Summary.Items.Pick),
	}
	defaultBuild.ItemBuilds = append(defaultBuild.ItemBuilds, mostCommonBuild)

	runeTitlePrefix := "[lolalytics]"
	runeTitleSuffix := "@" + curLane + ", " + sourceVersion + " (G+)"
	if aram {
		runeTitlePrefix = "[lolalytics-ARAM]"
		runeTitleSuffix = ", " + sourceVersion + " (G+)"
	}
	highestWinRune := common.RuneItem{
		Alias:           champion.Id,
		Name:            runeTitlePrefix + " Highest Win" + runeTitleSuffix,
		Position:        curLane,
		WinRate:         fmt.Sprintf("%v%%", resp.Summary.Runes.Win.Wr),
		SelectedPerkIds: concatRuneIds(resp.Summary.Runes.Win.Set.Pri, resp.Summary.Runes.Win.Set.Sec, resp.Summary.Runes.Win.Set.Mod),
		PrimaryStyleId:  common.GetPrimaryIdForRune(resp.Summary.Runes.Win.Set.Pri[0], runeLookUp),
		SubStyleId:      common.GetPrimaryIdForRune(resp.Summary.Runes.Win.Set.Sec[0], runeLookUp),
		PickCount:       resp.Summary.Runes.Win.N,
	}
	defaultBuild.Runes = append(defaultBuild.Runes, highestWinRune)
	mostCommonRune := common.RuneItem{
		Alias:           champion.Id,
		Name:            runeTitlePrefix + " Most Common" + runeTitleSuffix,
		Position:        curLane,
		WinRate:         fmt.Sprintf("%v%%", resp.Summary.Runes.Pick.Wr),
		SelectedPerkIds: concatRuneIds(resp.Summary.Runes.Pick.Set.Pri, resp.Summary.Runes.Pick.Set.Sec, resp.Summary.Runes.Pick.Set.Mod),
		PrimaryStyleId:  common.GetPrimaryIdForRune(resp.Summary.Runes.Pick.Set.Pri[0], runeLookUp),
		SubStyleId:      common.GetPrimaryIdForRune(resp.Summary.Runes.Pick.Set.Sec[0], runeLookUp),
		PickCount:       resp.Summary.Runes.Pick.N,
	}
	defaultBuild.Runes = append(defaultBuild.Runes, mostCommonRune)

	builds = append(builds, defaultBuild)

	if fetchMore && !aram {
		var restLanes []string
		for _, lane := range common.GetKeys(resp.Nav.Lanes) {
			if (lane != curLane) && (resp.Nav.Lanes[lane] >= MinimumPickRate) {
				restLanes = append(restLanes, lane)
			}
		}

		if len(restLanes) > 0 {
			wg := new(sync.WaitGroup)
			ch := make(chan []common.ChampionDataItem, len(restLanes))

			for _, l := range restLanes {
				wg.Add(1)

				go func(champion common.ChampionItem, query string, sourceVersion string, timestamp int64, cnt int, l string) {
					q := query + "&lane=" + l
					r, _ := makeBuild(champion, q, sourceVersion, officialVer, timestamp, cnt, false, runeLookUp, aram)
					if r != nil {
						ch <- *r
					}

					wg.Done()
				}(champion, query, sourceVersion, timestamp, cnt, l)
			}

			wg.Wait()
			close(ch)

			for d := range ch {
				builds = append(builds, d...)
			}
		}
	}

	additionalText := ""
	if aram {
		additionalText = "(ARAM mode)"
	}
	fmt.Printf("[lolalytics] No.%d Fetched: %s@%s %s\n", cnt, champion.Name, curLane, additionalText)
	return &builds, nil
}

func Import(championAliasList map[string]common.ChampionItem, officialVer string, timestamp int64, runeLookUp common.IRuneLookUp, aram bool, debug bool) string {
	start := time.Now()
	if aram {
		fmt.Println("🌉 [lolalytics-aram]: Start...")
	} else {
		fmt.Println("🌉 [lolalytics]: Start...")
	}

	buildUrl := "https://lolalytics.com/lol/rengar/build/"
	if aram {
		buildUrl = "https://lolalytics.com/lol/rengar/aram/build/"
	}
	// get initial patch version/ep etc.
	body, err := common.MakeRequest(buildUrl)
	if err != nil {
		return err.Error()
	}

	html := string(body)
	eps := epReg.FindAllStringSubmatch(html, -1) // "ep=champion&p=d&v=9&patch=11.9&cid=107&lane=default&tier=platinum_plus&queue=420&region=all"
	epQuery := eps[0][0]
	// sourceVersion := getSourceVersion(epQuery)
	sourceVersion := getPatchVersion(officialVer)
	queryMaker := makeQuery(epQuery)

	q := queryMaker("103", "middle", "gold_plus")
	tierList, err := getTierList(q)
	if err != nil {
		return err.Error()
	}

	cIds := make([]string, 0, len(tierList.Cid))
	for key := range tierList.Cid {
		cIds = append(cIds, key)
	}

	wg := new(sync.WaitGroup)
	cnt := 0
	ch := make(chan []common.ChampionDataItem, len(cIds))

	for _, cid := range cIds {
		if debug && cnt == 7 {
			break
		}

		if cnt > 0 && cnt%7 == 0 {
			fmt.Println(`🌉 Take a break...`)
			time.Sleep(time.Second * 5)
		}

		cnt += 1
		wg.Add(1)

		champion := getChampionById(cid, championAliasList)
		query := queryMaker(cid, "default", "gold_plus")

		go func() {
			builds, err := makeBuild(champion, query, sourceVersion, officialVer, timestamp, cnt, true, runeLookUp, aram)
			if err == nil {
				ch <- *builds
			}

			wg.Done()
		}()
	}

	wg.Wait()
	close(ch)

	var data [][]common.ChampionDataItem
	for i := range ch {
		data = append(data, i)
	}
	pkgName := `lolalytics`
	if aram {
		pkgName = `lolalytics-aram`
	}
	common.Write2Folder(data, pkgName, timestamp, sourceVersion, officialVer)

	duration := time.Since(start)
	if aram {
		return fmt.Sprintf("🟢 [lolalytics-aram] Finished, took: %s.", duration)
	}
	return fmt.Sprintf("🟢 [lolalytics] Finished, took: %s.", duration)
}
