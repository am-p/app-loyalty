package service

import (
	"clientesFrecuentes/internal/web"
)

func pagination(page, size int, total int64) web.Pagination {
	pages := int64(0)
	if total > 0 {
		pages = (total + int64(size) - 1) / int64(size)
	}
	return web.Pagination{Page: page, PageSize: size, TotalItems: total, TotalPages: pages}
}
