package server

import (
	"net/http"
	"strconv"
)

const defaultPageSize = 25

type Pagination struct {
	Page     int
	PageSize int
	HasPrev  bool
	HasNext  bool
	PrevPage int
	NextPage int
}

func pageFromRequest(r *http.Request, pageSize int) Pagination {
	if pageSize <= 0 {
		pageSize = defaultPageSize
	}
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	return Pagination{Page: page, PageSize: pageSize}
}

func (p Pagination) Paginate(totalFetched int) Pagination {
	if totalFetched > p.PageSize {
		p.HasNext = true
		p.NextPage = p.Page + 1
	}
	if p.Page > 1 {
		p.HasPrev = true
		p.PrevPage = p.Page - 1
	}
	return p
}

func (p Pagination) Offset() int {
	return (p.Page - 1) * p.PageSize
}

func (p Pagination) Limit() int {
	return p.PageSize
}

func buildQueryParams(params map[string]string) map[string]string {
	result := make(map[string]string)
	for k, v := range params {
		if v != "" {
			result[k] = v
		}
	}
	return result
}
