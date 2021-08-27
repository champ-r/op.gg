package opgg

import (
	"data-crawler/pkg/common"
	"fmt"
	"github.com/PuerkitoBio/goquery"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

func genData(alias string, id int, version string) (*common.ChampionDataItem, error) {
	url := AramSourceUrl + "/" + alias + "/statistics"

	doc, err := common.ParseHTML(url)
	if err != nil {
		log.Fatal(err)
	}

	d := common.ChampionDataItem{
		Alias: alias,
	}

	doc.Find(`.champion-overview__table--summonerspell > tbody:last-child .champion-stats__list .champion-stats__list__item span`).Each(func(_ int, selection *goquery.Selection) {
		s := selection.Text()
		d.Skills = append(d.Skills, s)
	})

	doc.Find(`.champion-overview__table--summonerspell > tbody`).First().Find(`img`).Each(func(_ int, selection *goquery.Selection) {
		src, _ := selection.Attr("src")
		s := common.MatchSpellName(src)
		if len(s) > 0 {
			d.Spells = append(d.Spells, s)
		}
	})

	build := common.ItemBuild{
		Title:               "[OP.GG-ARAM] " + alias + " " + version,
		AssociatedMaps:      []int{12},
		AssociatedChampions: []int{id},
		Map:                 "any",
		Mode:                "any",
		PreferredItemSlots:  []string{},
		Sortrank:            1,
		StartedFrom:         "blank",
		Type:                "custom",
	}

	// item builds
	doc.Find(`.champion-overview__table:nth-child(2) .champion-overview__row--first`).Each(func(blockIdx int, selection *goquery.Selection) {
		blockType := strings.TrimSpace(selection.Find(`th.champion-overview__sub-header`).Text())
		isRecommendedBuild := strings.Contains(strings.ToLower(blockType), `recommended builds`)

		if isRecommendedBuild {
			var firstBlock common.ItemBuildBlockItem

			pickCnt := strings.ReplaceAll(selection.Find(`td.champion-overview__stats--pick.champion-overview__border > span`).Text(), `,`, ``)
			winRate := selection.Find(`td.champion-overview__stats--win.champion-overview__border > strong`).Text()
			firstBlock.Type = `Recommended build: Pick ` + pickCnt + `, Win Rate ` + winRate
			selection.Find("li.champion-stats__list__item img").Each(func(i int, img *goquery.Selection) {
				src, _ := img.Attr("src")
				id := common.MatchId(src)
				firstBlock.Items = append(firstBlock.Items, common.BlockItem{
					Id:    id,
					Count: 1,
				})
			})

			build.Blocks = append(build.Blocks, firstBlock)

			selection.NextUntil(`tr.champion-overview__row--first`).Each(func(trIdx int, tr *goquery.Selection) {
				pickCnt := strings.ReplaceAll(tr.Find(`td.champion-overview__stats--pick.champion-overview__border > span`).Text(), `,`, ``)
				winRate := tr.Find(`td.champion-overview__stats--win.champion-overview__border > strong`).Text()

				var block common.ItemBuildBlockItem
				block.Type = `Recommended build: Pick ` + pickCnt + `, Win Rate ` + winRate

				tr.Find("li.champion-stats__list__item img").Each(func(i int, img *goquery.Selection) {
					src, _ := img.Attr("src")
					id := common.MatchId(src)
					block.Items = append(block.Items, common.BlockItem{
						Id:    id,
						Count: 1,
					})
				})

				build.Blocks = append(build.Blocks, block)
			})

			return
		}

		var block common.ItemBuildBlockItem
		block.Type = blockType

		var itemIds []string
		selection.Find("li.champion-stats__list__item img").Each(func(i int, img *goquery.Selection) {
			src, _ := img.Attr("src")
			id := common.MatchId(src)
			itemIds = common.NoRepeatPush(id, itemIds)
		})
		selection.NextUntil(`tr.champion-overview__row--first`).Find("li.champion-stats__list__item img").Each(func(_ int, img *goquery.Selection) {
			src, _ := img.Attr("src")
			id := common.MatchId(src)
			itemIds = common.NoRepeatPush(id, itemIds)
		})

		for _, val := range itemIds {
			item := common.BlockItem{
				Id:    val,
				Count: 1,
			}
			block.Items = append(block.Items, item)
		}
		build.Blocks = append(build.Blocks, block)
	})

	// consumables
	b := common.ItemBuildBlockItem{
		Type: "Consumables",
	}
	for _, id := range common.ConsumableItems {
		item := common.BlockItem{
			Id:    id,
			Count: 1,
		}
		b.Items = append(b.Items, item)
	}
	build.Blocks = append(build.Blocks, b)

	d.ItemBuilds = append(d.ItemBuilds, build)

	// runes
	doc.Find(`[class*=ChampionKeystoneRune] tr`).Each(func(_ int, tr *goquery.Selection) {
		var runeItem common.RuneItem
		runeItem.Alias = alias

		tr.Find(`.perk-page__item--active img`).Each(func(_ int, img *goquery.Selection) {
			src, _ := img.Attr(`src`)
			sId, _ := strconv.Atoi(common.MatchId(src))
			runeItem.SelectedPerkIds = append(runeItem.SelectedPerkIds, sId)
		})

		tr.Find(`.fragment__detail img.active`).Each(func(_ int, img *goquery.Selection) {
			src, _ := img.Attr(`src`)
			fId, _ := strconv.Atoi(common.MatchId(src))
			runeItem.SelectedPerkIds = append(runeItem.SelectedPerkIds, fId)
		})

		pIdSrc, _ := tr.Find(`.perk-page__item--mark img`).First().Attr(`src`)
		runeItem.PrimaryStyleId, _ = strconv.Atoi(common.MatchId(pIdSrc))

		sIdSrc, _ := tr.Find(`.perk-page__item--mark img`).Last().Attr(`src`)
		runeItem.SubStyleId, _ = strconv.Atoi(common.MatchId(sIdSrc))

		pickCount := tr.Find(`.champion-overview__stats--pick .pick-ratio__text`).Next().Next().Text()
		runeItem.PickCount, _ = strconv.Atoi(strings.ReplaceAll(pickCount, `,`, ``))
		runeItem.WinRate = tr.Find(`.champion-overview__stats--pick .win-ratio__text`).Next().Text()

		runeItem.Name = "[OP.GG-ARAM] " + alias + " - " + runeItem.WinRate + ", " + fmt.Sprint(runeItem.PickCount)

		d.Runes = append(d.Runes, runeItem)
	})

	sort.Slice(d.Runes, func(i, j int) bool {
		return d.Runes[i].PickCount > d.Runes[j].PickCount
	})

	return &d, nil
}

func startJob(champ ChampionListItem, index int, version string) *common.ChampionDataItem {
	time.Sleep(time.Second * 1)

	alias := champ.Alias
	// fmt.Printf("⌛ [OP.GG-ARAM]️️ No.%d, %s @ %s\n", index, alias, position)

	id, _ := strconv.Atoi(champ.Id)
	d, _ := genData(alias, id, version)
	if d != nil {
		d.Index = index
		d.Id = champ.Id
		d.Name = champ.Name
	}

	fmt.Printf("🌟 [OP.GG-ARAM] No.%d, %s \n", index, alias)
	return d
}

func ImportAram(allChampions map[string]common.ChampionItem, aliasList map[string]string, officialVer string, timestamp int64, debug bool) string {
	start := time.Now()
	fmt.Println("🤖 [OP.GG-ARAM] Start...")

	d, count := genOverview(allChampions, aliasList, true)
	fmt.Printf("🤪 [OP.GG-ARAM] Got champions & positions, count: %d \n", count)

	wg := new(sync.WaitGroup)
	cnt := 0
	ch := make(chan common.ChampionDataItem, count)

listLoop:
	for _, cur := range d.ChampionList {
		cnt += 1

		if cnt%7 == 0 {
			if debug {
				wg.Done()
				break listLoop
			}
			time.Sleep(time.Second * 5)
		}

		wg.Add(1)
		go func(_cur ChampionListItem, _cnt int, _ver string) {
			ch <- *startJob(_cur, _cnt, _ver)
			wg.Done()
		}(cur, cnt, d.Version)
	}

	wg.Wait()
	close(ch)

	outputPath := filepath.Join(".", "output", AramPkgName)
	_ = os.MkdirAll(outputPath, os.ModePerm)

	failed := 0
	r := make(map[string][]common.ChampionDataItem)

	for champion := range ch {
		if champion.Skills != nil {
			champion.Timestamp = timestamp
			champion.Version = d.Version
			champion.OfficialVersion = officialVer
			r[champion.Alias] = append(r[champion.Alias], champion)
		}
	}

	for k, v := range r {
		fileName := outputPath + "/" + k + ".json"
		_ = common.SaveJSON(fileName, v)
	}

	_ = common.SaveJSON("output/index.json", allChampions)

	pkg, _ := common.GenPkgInfo("tpl/package.json", common.PkgInfo{
		Timestamp:       timestamp,
		SourceVersion:   d.Version,
		OfficialVersion: officialVer,
		PkgName:         AramPkgName,
	})
	_ = ioutil.WriteFile("output/"+AramPkgName+"/package.json", []byte(pkg), 0644)

	duration := time.Since(start)
	return fmt.Sprintf("🟢 [OP.GG-ARAM] All finished, success: %d, failed: %d, took %s", cnt-failed, failed, duration)
}
