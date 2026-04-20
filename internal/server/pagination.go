package server

import (
	"net/http"
	"strconv"
)

const defaultPageSize = 25

type Pagination struct {
	Offset    int
	Limit     int
	HasMore   bool
	NextOffset int
}

func pageFromRequest(r *http.Request, limit int) Pagination {
	if limit <= 0 {
		limit = defaultPageSize
	}
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	if offset < 0 {
		offset = 0
	}
	return Pagination{Offset: offset, Limit: limit}
}

func (p Pagination) Paginate(count int) Pagination {
	p.HasMore = count > p.Limit
	p.NextOffset = p.Offset + p.Limit
	return p
}

func (p Pagination) FetchLimit() int {
	return p.Limit + 1
}
