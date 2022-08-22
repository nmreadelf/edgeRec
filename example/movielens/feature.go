package movielens

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"sync"

	_ "github.com/mattn/go-sqlite3"
	log "github.com/sirupsen/logrus"

	"github.com/auxten/edgeRec/feature"
	"github.com/auxten/edgeRec/nn/base"
	rcmd "github.com/auxten/edgeRec/recommend"
	"github.com/auxten/edgeRec/utils"
)

var (
	dbOnce    sync.Once
	db        *sql.DB
	yearRegex = regexp.MustCompile(`\((\d{4})\)$`)
)

func initDb(dbPath string) (err error) {
	dbOnce.Do(func() {
		db, err = sql.Open("sqlite3", dbPath)
		if err != nil {
			log.Errorf("failed to open db: %v", err)
			return
		}
	})
	return
}

type RecSysImpl struct {
	DataPath   string
	SampleCnt  int
	Neural     base.Predicter
	mRatingMap map[int][2]float64
}

func (recSys *RecSysImpl) ItemSeqGenerator(ctx context.Context) (ret <-chan string, err error) {
	var (
		wg sync.WaitGroup
	)
	wg.Add(1)
	ch := make(chan string, 100)
	go func() {
		var (
			i    int
			rows *sql.Rows
		)
		defer func() {
			log.Debugf("item seq generator finished: %d", i)
			close(ch)
		}()
		// predict must use the same embedding as train
		rows, err = db.Query("SELECT movieId FROM ratings_train r WHERE r.rating > 3.5 order by userId, timestamp")
		if err != nil {
			log.Errorf("failed to query ratings: %v", err)
			wg.Done()
			return
		}
		wg.Done()
		defer rows.Close()
		for rows.Next() {
			i++
			var movieId sql.NullInt64
			if err = rows.Scan(&movieId); err != nil {
				log.Errorf("failed to scan movieId: %v", err)
				continue
			}
			ch <- fmt.Sprintf("%d", movieId.Int64)
		}
	}()

	wg.Wait()
	ret = ch
	return
}

func (recSys *RecSysImpl) GetItemsFeature(ctx context.Context, itemIds []int) (tensors []rcmd.Tensor, err error) {
	if len(itemIds) == 0 {
		return nil, fmt.Errorf("GetItemsFeature fail, should not provide empty item ids")
	}
	// get movie avg rating and rating count
	var (
		rows *sql.Rows
	)
	var ids []string
	for _, i := range itemIds {
		ids = append(ids, strconv.Itoa(i))
	}
	var idToTensor = make(map[int]rcmd.Tensor)

	rows, err = db.Query(`select m."movieId" itemId,
					   "title"     itemTitle,
					   "genres"    itemGenres
				from movies m
				WHERE m.movieId IN (?)`, strings.Join(ids, ","))
	if err != nil {
		log.Errorf("failed to query ratings: %v", err)
		return
	}
	defer rows.Close()
	for rows.Next() {
		var (
			itemId, movieYear     int
			itemTitle, itemGenres string
			avgRating, cntRating  float64
			GenreTensor           [50]float64 // 5 * 10
			tensor                rcmd.Tensor
		)
		if err = rows.Scan(&itemId, &itemTitle, &itemGenres); err != nil {
			log.Errorf("failed to scan movieId: %v", err)
			return
		}
		// regex match year from itemTitle
		yearStrSlice := yearRegex.FindStringSubmatch(itemTitle)
		if len(yearStrSlice) > 1 {
			movieYear, err = strconv.Atoi(yearStrSlice[1])
			if err != nil {
				log.Errorf("failed to parse year: %v", err)
				return
			}
		}
		// itemGenres
		genres := strings.Split(itemGenres, "|")
		for i, genre := range genres {
			if i >= 5 {
				break
			}
			copy(GenreTensor[i*10:], genreFeature(genre))
		}
		if mr, ok := recSys.mRatingMap[itemId]; ok {
			avgRating = mr[0] / 5.
			cntRating = math.Log2(mr[1])
		}

		tensor = utils.ConcatSlice(tensor, GenreTensor[:], rcmd.Tensor{
			float64(movieYear-1990) / 20.0, avgRating, cntRating,
		})
		idToTensor[itemId] = tensor
	}
	for _, i := range itemIds {
		v, ok := idToTensor[i]
		if !ok {
			return nil, fmt.Errorf("item: %d not found", i)
		}
		tensors = append(tensors, v)
	}

	return
}

func (recSys *RecSysImpl) GetItemFeature(ctx context.Context, itemId int) (tensor rcmd.Tensor, err error) {
	// get movie avg rating and rating count
	var (
		rows *sql.Rows
	)

	rows, err = db.Query(`select m."movieId" itemId,
					   "title"     itemTitle,
					   "genres"    itemGenres
				from movies m
				WHERE m.movieId = ?`, itemId)
	if err != nil {
		log.Errorf("failed to query ratings: %v", err)
		return
	}
	defer rows.Close()
	if rows.Next() {
		var (
			itemId, movieYear     int
			itemTitle, itemGenres string
			avgRating, cntRating  float64
			GenreTensor           [50]float64 // 5 * 10
		)
		if err = rows.Scan(&itemId, &itemTitle, &itemGenres); err != nil {
			log.Errorf("failed to scan movieId: %v", err)
			return
		}
		// regex match year from itemTitle
		yearStrSlice := yearRegex.FindStringSubmatch(itemTitle)
		if len(yearStrSlice) > 1 {
			movieYear, err = strconv.Atoi(yearStrSlice[1])
			if err != nil {
				log.Errorf("failed to parse year: %v", err)
				return
			}
		}
		// itemGenres
		genres := strings.Split(itemGenres, "|")
		for i, genre := range genres {
			if i >= 5 {
				break
			}
			copy(GenreTensor[i*10:], genreFeature(genre))
		}
		if mr, ok := recSys.mRatingMap[itemId]; ok {
			avgRating = mr[0] / 5.
			cntRating = math.Log2(mr[1])
		}

		tensor = utils.ConcatSlice(tensor, GenreTensor[:], rcmd.Tensor{
			float64(movieYear-1990) / 20.0, avgRating, cntRating,
		})
		return
	} else {
		err = fmt.Errorf("itemId %d not found", itemId)
		return
	}
}

func (recSys *RecSysImpl) GetUserFeature(ctx context.Context, userId int) (tensor rcmd.Tensor, err error) {
	var (
		tableName        string
		rows, rows2      *sql.Rows
		genres           string
		avgRating        sql.NullFloat64
		cntRating        sql.NullFloat64
		top5GenresTensor [50]float64
	)
	// get stage value from ctx
	stage := ctx.Value(rcmd.StageKey).(rcmd.Stage)
	switch stage {
	case rcmd.TrainStage:
		tableName = "ratings_train"
	case rcmd.PredictStage:
		tableName = "ratings_test"
	default:
		panic("unknown stage")
	}

	rows, err = db.Query(`select 
                           group_concat(genres) as ugenres
                    from `+tableName+` r2
                             left join movies t2 on r2.movieId = t2.movieId
                    where userId = ? and
                    		r2.rating > 3.5
                    group by userId`, userId)
	if err != nil {
		log.Errorf("failed to query ratings: %v", err)
		return
	}
	defer rows.Close()
	for rows.Next() {
		if err = rows.Scan(&genres); err != nil {
			log.Errorf("failed to scan movieId: %v", err)
			return
		}
	}

	genreList := strings.Split(genres, ",|")
	top5Genres := utils.TopNOccurrences(genreList, 5)
	for i, genre := range top5Genres {
		copy(top5GenresTensor[i*10:], genreFeature(genre.Key))
	}

	rows2, err = db.Query(`select avg(rating) as avgRating, 
						   count(rating) cntRating
					from `+tableName+` where userId = ?`, userId)
	if err != nil {
		log.Errorf("failed to query ratings: %v", err)
		return
	}
	defer rows2.Close()
	for rows2.Next() {
		if err = rows2.Scan(&avgRating, &cntRating); err != nil {
			log.Errorf("failed to scan movieId: %v", err)
			return
		}
	}

	tensor = utils.ConcatSlice(rcmd.Tensor{avgRating.Float64 / 5., cntRating.Float64 / 100.}, top5GenresTensor[:])
	return
}

func genreFeature(genre string) (tensor rcmd.Tensor) {
	return feature.HashOneHot([]byte(genre), 10)
}

func (recSys *RecSysImpl) SampleGenerator(_ context.Context) (ret <-chan rcmd.Sample, err error) {
	sampleCh := make(chan rcmd.Sample, 10000)
	var (
		wg sync.WaitGroup
	)
	wg.Add(1)
	go func() {
		var (
			i    int
			rows *sql.Rows
		)
		defer func() {
			log.Debugf("sample generator finished: %d", i)
			close(sampleCh)
		}()

		rows, err = db.Query(
			"SELECT userId, movieId, rating FROM ratings_train ORDER BY timestamp, userId ASC LIMIT ?", recSys.SampleCnt)
		if err != nil {
			log.Errorf("failed to query ratings: %v", err)
			wg.Done()
			return
		}
		wg.Done()
		defer rows.Close()
		for rows.Next() {
			i++
			var (
				userId, movieId int
				rating, label   float64
			)
			if err = rows.Scan(&userId, &movieId, &rating); err != nil {
				log.Errorf("failed to scan ratings: %v", err)
				return
			}
			label = BinarizeLabel(rating)
			// label = rating / 5.0

			sampleCh <- rcmd.Sample{
				UserId: userId,
				ItemId: movieId,
				Label:  label,
			}
		}
	}()

	wg.Wait()
	ret = sampleCh
	return
}

func (recSys *RecSysImpl) PreTrain(ctx context.Context) (err error) {
	if err = initDb(recSys.DataPath); err != nil {
		return
	}
	// get movie avg rating and rating count
	var (
		rows1 *sql.Rows
	)
	rows1, err = db.Query(`select movieId, avg(rating) avg_r, count(rating) cnt_r
                    from ratings_train
                    group by movieId`)
	if err != nil {
		log.Errorf("failed to query ratings: %v", err)
		return
	}
	defer rows1.Close()
	recSys.mRatingMap = make(map[int][2]float64)
	for rows1.Next() {
		var (
			movieId int
			avgR    float64
			cntR    int
		)
		if err = rows1.Scan(&movieId, &avgR, &cntR); err != nil {
			log.Errorf("failed to scan movieId: %v", err)
			return
		}
		recSys.mRatingMap[movieId] = [2]float64{avgR, float64(cntR)}
	}

	return
}

func BinarizeLabel(rating float64) float64 {
	if rating > 3.5 {
		return 1.0
	}
	return 0.0
}
