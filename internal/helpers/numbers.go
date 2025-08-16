package helpers

import "time"

func GetNextMemberNumber() int {
	return int(time.Now().Unix()) % 1000000
}
