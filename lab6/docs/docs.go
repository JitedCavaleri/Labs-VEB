// Package docs — заглушка, реальное содержимое генерируется командой `swag init`
// при сборке Docker-образа (см. Dockerfile). Этот файл нужен только для того,
// чтобы main.go компилировался в IDE до запуска `swag init`.
//
// После `swag init` файл будет перезаписан реальной спецификацией.
//
// Лаба 6: к описаниям из лаб 2-5 добавлены аннотации об ObjectID и MongoDB,
// см. main.go @description блок.
package docs

import "github.com/swaggo/swag"

// SwaggerInfo — минимальный плейсхолдер. После `swag init` он заменится
// на структуру с автогенерированным OpenAPI JSON.
var SwaggerInfo = &swag.Spec{
	Version:          "1.0",
	Host:             "localhost:4200",
	BasePath:         "/",
	Schemes:          []string{"http"},
	Title:            "Lab Project API (placeholder — run `swag init`)",
	Description:      "Запустите `swag init` или соберите Docker-образ — этот файл будет перезаписан.",
	InfoInstanceName: "swagger",
	SwaggerTemplate:  `{"swagger":"2.0","info":{"title":"placeholder","version":"1.0"},"paths":{}}`,
	LeftDelim:        "{{",
	RightDelim:       "}}",
}

func init() {
	swag.Register(SwaggerInfo.InstanceName(), SwaggerInfo)
}
