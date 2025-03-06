package utils

import "github.com/google/uuid"

func GetUuid() (string, error) {
	newUUID, err := uuid.NewUUID()
	if err != nil {
		return "", err
	}
	return newUUID.String(), nil
}
