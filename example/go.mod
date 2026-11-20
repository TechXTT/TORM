module github.com/TechXTT/TORM/example

go 1.23.4

require (
	github.com/TechXTT/TORM/v2 v2.0.0
	github.com/google/uuid v1.6.0
	github.com/joho/godotenv v1.5.1
)

require github.com/lib/pq v1.10.9 // indirect

replace github.com/TechXTT/TORM/v2 => ..
