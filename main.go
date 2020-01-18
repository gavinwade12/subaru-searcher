package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/smtp"
	"os"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

type vehicle struct {
	Make          string
	Model         string
	Year          string
	VIN           string
	Color         string
	Mileage       string
	EngineSize    string
	Row           string
	VehicleNumber string
	Description   string
	Site          string
}

const dbFile = ".subarus"

func main() {
	subarus, err := fetchCherryPickedSubarus()
	if err != nil {
		log.Fatal(err)
	}

	s, err := fetchLKQSubarus()
	if err != nil {
		log.Fatal(err)
	}
	subarus = append(subarus, s...)

	savedSubarus, err := loadSavedSubarus()
	if err != nil {
		log.Fatal(err)
	}

	newSubarus := []vehicle{}
	for _, v := range subarus {
		saved := false
		for _, sv := range savedSubarus {
			if sv.VIN == v.VIN {
				saved = true
				break
			}
		}
		if saved {
			continue
		}

		newSubarus = append(newSubarus, v)
		savedSubarus = append(savedSubarus, v)
	}

	if len(newSubarus) == 0 {
		return
	}

	from := os.Getenv("SUBARU_NOTIF_FROM")
	to := os.Getenv("SUBARU_NOTIF_TO")
	pass := os.Getenv("SUBARU_NOTIF_PASS")
	if err = sendNotificationOfNewSubarus(from, to, pass, newSubarus); err != nil {
		log.Fatal(err)
	}

	if err = saveSubarus(savedSubarus); err != nil {
		log.Fatal(err)
	}
}

func fetchCherryPickedSubarus() ([]vehicle, error) {
	res, err := http.Get("https://www.cherrypickedparts.com/_ext2/inventory/")
	if err != nil {
		return nil, err
	}

	defer res.Body.Close()
	if res.StatusCode != 200 {
		return nil, fmt.Errorf("invalid response status code: %d", res.StatusCode)
	}

	var response [][]string
	if err = json.NewDecoder(res.Body).Decode(&response); err != nil {
		return nil, err
	}

	subarus := []vehicle{}
	for _, v := range response {
		if strings.ToLower(v[3]) != "subaru" {
			continue
		}

		subarus = append(subarus, vehicle{
			Make:          v[3],
			Model:         v[4],
			Year:          v[2],
			VIN:           v[8],
			Color:         v[5],
			Mileage:       v[9],
			EngineSize:    v[7],
			Row:           v[0],
			VehicleNumber: v[1],
			Description:   v[6],
			Site:          "Cherry Picked Auto Parts",
		})
	}

	return subarus, nil
}

func fetchLKQSubarus() ([]vehicle, error) {
	req, err := http.NewRequest(http.MethodGet, "https://www.lkqpickyourpart.com/DesktopModules/pyp_vehicleInventory/getVehicleInventory.aspx?store=259&page=0&filter=subaru&carbuyYardCode=1259&pageSize=25&language=en-US&thumbQ=60&fullQ=70", nil)
	if err != nil {
		return nil, err
	}

	req.Header.Add("referer", "https://www.lkqpickyourpart.com/locations/LKQ_Pick_Your_Part_-_Toledo-259/recents/")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}

	defer res.Body.Close()
	if res.StatusCode != 200 {
		return nil, fmt.Errorf("invalid response status code: %d", res.StatusCode)
	}

	ctx := &html.Node{
		Type:     html.ElementNode,
		DataAtom: atom.Table,
		Data:     "table",
	}

	ns, err := html.ParseFragment(res.Body, ctx)
	if err != nil {
		return nil, err
	}
	for _, n := range ns {
		ctx.AppendChild(n)
	}

	subarus := []vehicle{}
	goquery.NewDocumentFromNode(ctx).Find("tr.pypvi_resultRow").Each(func(_ int, s *goquery.Selection) {
		v := vehicle{
			Make: s.Find("td.pypvi_make").Contents().FilterFunction(func(_ int, s *goquery.Selection) bool {
				return s.Get(0).Type == html.TextNode
			}).Text(),
			Model:       s.Find("td.pypvi_model").Text(),
			Year:        s.Find("td.pypvi_year").Text(),
			Description: "Available On: " + s.Find("td.pypvi_date").Text(),
			Site:        "LKQ Pick Your Part",
		}

		s.Find("td.pypvi_make > div.pypvi_notes > p").Each(func(i int, s *goquery.Selection) {
			switch i {
			case 1:
				v.Row = strings.Replace(s.Text(), "Row: ROW ", "", -1)
				break
			case 2:
				v.VehicleNumber = strings.Replace(s.Text(), "Space: ", "", -1)
				break
			case 3:
				v.Color = strings.Replace(s.Text(), "Color: ", "", -1)
				break
			case 4:
				v.VIN = strings.Replace(s.Text(), "VIN: ", "", -1)
				break
			}
		})

		subarus = append(subarus, v)
	})

	return subarus, nil
}

func loadSavedSubarus() ([]vehicle, error) {
	f, err := os.Open(dbFile)
	if err != nil {
		if os.IsNotExist(err) {
			return []vehicle{}, nil
		}
		return nil, err
	}
	defer f.Close()

	subarus := []vehicle{}
	if err = json.NewDecoder(f).Decode(&subarus); err != nil {
		return nil, err
	}

	return subarus, nil
}

func saveSubarus(subarus []vehicle) error {
	f, err := os.Create(dbFile)
	if err != nil {
		return err
	}
	defer f.Close()

	return json.NewEncoder(f).Encode(subarus)
}

func sendNotificationOfNewSubarus(from, to, pass string, subarus []vehicle) error {
	if from == "" {
		return fmt.Errorf("'from' required to send email notification")
	}
	if pass == "" {
		return fmt.Errorf("'pass' required to send email notification")
	}
	if to == "" {
		to = from
	}

	auth := smtp.PlainAuth("", from, pass, "smtp.gmail.com")
	b, err := json.MarshalIndent(subarus, "", "  ")
	if err != nil {
		return err
	}
	msg := "From: " + from + "\nTo: " + to + "\nSubject: New Subarus Found!\n\n" + string(b)
	return smtp.SendMail("smtp.gmail.com:587", auth, from, []string{to}, []byte(msg))
}
