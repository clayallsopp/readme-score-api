// +build !heroku

package main

func HandleError(err error) {
	if err != nil {
		panic(err)
	}
}
