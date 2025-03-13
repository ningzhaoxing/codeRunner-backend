package service

import "codeRunner-siwu/internal/domain/server/entity"

type ClientManagerDomain interface {
	Add(*entity.Client, int64)
	Remove(string)
	GetServerByBalance() (*entity.Client, error)
}
