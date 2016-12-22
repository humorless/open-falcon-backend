package http

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"gopkg.in/gin-gonic/gin.v1"
	ogin "github.com/Cepave/open-falcon-backend/common/gin"

	nqmDb "github.com/Cepave/open-falcon-backend/common/db/nqm"

	dsl "github.com/Cepave/open-falcon-backend/modules/query/dsl/nqm_parser"
	"github.com/Cepave/open-falcon-backend/modules/query/nqm"
	"github.com/bitly/go-simplejson"
)

const (
	jsonIndent = false
	jsonCoding = false
)

var nqmService *nqm.ServiceController

// Although these services use Gin framework, the configuration depends on "http.listen" property,
// not "gin_http.listen"
func configNqmRoutes() {
	nqmService = nqm.GetDefaultServiceController()
	nqmService.Init()

	http.Handle("/nqm/", getGinRouter())
}

func getGinRouter() *gin.Engine {
	engine := ogin.NewDefaultJsonEngine(&ogin.GinConfig{ Mode: gin.ReleaseMode })

	engine.GET("/nqm/icmp/list/by-provinces", listIcmpByProvinces)
	engine.GET("/nqm/icmp/province/:province_id/list/by-targets", listIcmpByTargetsForAProvince)
	engine.GET("/nqm/province/:province_id/agents", listEffectiveAgentsInProvince)

	engine.GET("/nqm/icmp/compound-report/:query_id", outputCompondReportOfIcmp)
	engine.POST("/nqm/icmp/compound-report", setupCompondReportOfIcmp)

	return engine
}

func setupCompondReportOfIcmp(context *gin.Context) {
}
func outputCompondReportOfIcmp(context *gin.Context) {
}

type resultWithDsl struct {
	queryParams *dsl.QueryParams
	resultData interface{}
}

func (result *resultWithDsl) MarshalJSON() ([]byte, error) {
	jsonObject := simplejson.New()

	jsonObject.SetPath([]string{ "dsl", "start_time" }, result.queryParams.StartTime.Unix())
	jsonObject.SetPath([]string{ "dsl", "end_time" }, result.queryParams.EndTime.Unix())
	jsonObject.Set("result", result.resultData)

	return jsonObject.MarshalJSON()
}

// Lists agents(grouped by city) for a province
func listEffectiveAgentsInProvince(context *gin.Context) {
	provinceId, err := strconv.ParseInt(context.Param("province_id"), 10, 16)
	if err != nil {
		panic(err)
	}

	context.JSON(
		http.StatusOK,
		nqmDb.LoadEffectiveAgentsInProvince(int16(provinceId)),
	)
}

// Lists statistics data of ICMP, which would be grouped by provinces
func listIcmpByProvinces(context *gin.Context) {
	dslParams, isValid := processDslAndOutputError(context, context.Query("dsl"))
	if !isValid {
		return
	}

	context.JSON(
		http.StatusOK,
		&resultWithDsl{
			queryParams: dslParams,
			resultData: nqmService.ListByProvinces(dslParams),
		},
	)
}

// Lists data of targets, which would be grouped by cities
func listIcmpByTargetsForAProvince(context *gin.Context) {
	dslParams, isValid := processDslAndOutputError(context, context.Query("dsl"))
	if !isValid {
		return
	}

	dslParams.AgentFilter.MatchProvinces = make([]string, 0) // Ignores the province of agent

	provinceId, _ := strconv.ParseInt(context.Param("province_id"), 10, 16)
	dslParams.AgentFilterById.MatchProvinces = []int16 { int16(provinceId) } // Use the id as the filter of agent

	if agentId, parseErrForAgentId := strconv.ParseInt(context.Query("agent_id"), 10, 16)
		parseErrForAgentId == nil {
		dslParams.AgentFilterById.MatchIds = []int32 { int32(agentId) } // Set the filter by agent's id
	} else if cityId, parseErrForCityId := strconv.ParseInt(context.Query("city_id_of_agent"), 10, 16)
		parseErrForCityId == nil {
		dslParams.AgentFilterById.MatchCities = []int16 { int16(cityId) } // Set the filter by city's id
	}

	context.JSON(
		http.StatusOK,
		&resultWithDsl{
			queryParams: dslParams,
			resultData: nqmService.ListTargetsWithCityDetail(dslParams),
		},
	)
}

type jsonDslError struct {
	Code int `json:"error_code"`
	Message string `json:"error_message"`
}
func outputDslError(context *gin.Context, err error) {
	context.JSON(
		http.StatusBadRequest,
		&jsonDslError {
			Code: 1,
			Message: err.Error(),
		},
	)
}

const (
	defaultDaysForTimeRange = 7
	after7Days = defaultDaysForTimeRange * 24 * time.Hour
	before7Days = after7Days * -1
)

// Process DSL and output error
// Returns: true if the DSL is valid
func processDslAndOutputError(context *gin.Context, dslText string) (*dsl.QueryParams, bool) {
	dslParams, err := processDsl(dslText)
	if err == nil {
		return dslParams, true
	}

	context.JSON(
		http.StatusBadRequest,
		&struct {
			Code int `json:"error_code"`
			Message string `json:"error_message"`
		} {
			Code: 1,
			Message: err.Error(),
		},
	)

	return nil, false
}

// The query of DSL would be inner-province(used for phase 1)
func processDsl(dslParams string) (*dsl.QueryParams, error) {
	strNqmDsl := strings.TrimSpace(dslParams)

	/**
	 * If any of errors for parsing DSL
	 */
	paramSetters, parseError := dsl.Parse(
		"Query.nqmdsl", []byte(strNqmDsl),
	)
	if parseError != nil {
		return nil, parseError
	}
	// :~)

	queryParams := dsl.NewQueryParams()
	queryParams.SetUpParams(paramSetters)

	setupTimeRange(queryParams)
	setupInnerProvince(queryParams)

	paramsError := queryParams.CheckRationalOfParameters()
	if paramsError != nil {
		return nil, paramsError
	}

	return queryParams, nil
}

// Sets-up the time range with provided-or-not value of parameters
// 1. Without any parameter of time range
// 2. Has only start time
// 3. Has only end time
func setupTimeRange(queryParams *dsl.QueryParams) {
	if queryParams.StartTime.IsZero() && queryParams.EndTime.IsZero() {
		now := time.Now()

		queryParams.StartTime = now.Add(before7Days) // Include 7 days before
		queryParams.EndTime = now.Add(24 * time.Hour) // Include today
		return
	}

	if queryParams.StartTime.IsZero() && !queryParams.EndTime.IsZero() {
		queryParams.StartTime = queryParams.EndTime.Add(before7Days)
		return
	}

	if !queryParams.StartTime.IsZero() && queryParams.EndTime.IsZero() {
		queryParams.EndTime = queryParams.StartTime.Add(after7Days)
		return
	}

	if queryParams.StartTime.Unix() == queryParams.EndTime.Unix() {
		queryParams.EndTime = queryParams.StartTime.Add(24 * time.Hour)
	}
}

/**
 * !IMPORTANT!
 * This default value is just used in phase 1 funcion of NQM reporting(inner-province)
 */
func setupInnerProvince(queryParams *dsl.QueryParams) {
	queryParams.ProvinceRelation = dsl.SAME_VALUE
}
