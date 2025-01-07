package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/gorilla/mux"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

// Struct Retur untuk merepresentasikan data retur yang ada di database
type Retur struct {
	ID          int    `json:"id"`
	Barang      string `json:"barang"`
	Alasan      string `json:"alasan"`
	Status      string `json:"status"`
	Pengembalian string `json:"pengembalian"`
}

// Stack adalah implementasi stack generik dengan tipe data generik (T)
type Stack[T any] struct {
	items []T
}

func (s *Stack[T]) Push(item T) {
	s.items = append(s.items, item)
}

func (s *Stack[T]) Pop() (T, bool) {
	if len(s.items) == 0 {
		var zero T
		return zero, false
	}
	item := s.items[len(s.items)-1]
	s.items = s.items[:len(s.items)-1]
	return item, true
}

func (s *Stack[T]) IsEmpty() bool {
	return len(s.items) == 0
}

var (
	db           *gorm.DB
	deletedStack Stack[Retur]
	deletedIDs   []int // Menyimpan ID barang yang dihapus
)

func initDB() {
	var err error
	dsn := "root:@tcp(127.0.0.1:3306)/retur_db?charset=utf8mb4&parseTime=True&loc=Local"
	db, err = gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		panic("Failed to connect to database: " + err.Error())
	}
	db.AutoMigrate(&Retur{})
}

func respondJSON(w http.ResponseWriter, status int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(payload)
}

func handleError(w http.ResponseWriter, status int, message string) {
	respondJSON(w, status, map[string]string{"error": message})
}

func getReturs(w http.ResponseWriter, r *http.Request) {
	var returs []Retur
	if err := db.Find(&returs).Error; err != nil {
		handleError(w, http.StatusInternalServerError, "Failed to retrieve returns")
		return
	}
	respondJSON(w, http.StatusOK, returs)
}

func createRetur(w http.ResponseWriter, r *http.Request) {
	var newRetur Retur
	if err := json.NewDecoder(r.Body).Decode(&newRetur); err != nil {
		handleError(w, http.StatusBadRequest, "Invalid input")
		return
	}

	if len(deletedIDs) > 0 {
		newRetur.ID = deletedIDs[len(deletedIDs)-1]
		deletedIDs = deletedIDs[:len(deletedIDs)-1]
	} else {
		var lastRetur Retur
		if err := db.Order("id desc").First(&lastRetur).Error; err == nil {
			newRetur.ID = lastRetur.ID + 1
		} else {
			newRetur.ID = 1
		}
	}

	newRetur.Status = "Dalam Proses"
	if err := db.Create(&newRetur).Error; err != nil {
		handleError(w, http.StatusInternalServerError, "Failed to create return")
		return
	}
	respondJSON(w, http.StatusCreated, newRetur)
}

func approveReturHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id, err := strconv.Atoi(vars["id"])
	if err != nil {
		handleError(w, http.StatusBadRequest, "Invalid ID format")
		return
	}

	var input struct {
		Pengembalian string `json:"pengembalian"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		handleError(w, http.StatusBadRequest, "Invalid input")
		return
	}

	if input.Pengembalian != "barang" && input.Pengembalian != "uang" {
		handleError(w, http.StatusBadRequest, "Pengembalian must be 'barang' or 'uang'")
		return
	}

	var retur Retur
	if err := db.First(&retur, id).Error; err != nil {
		handleError(w, http.StatusNotFound, "Return not found")
		return
	}

	retur.Pengembalian = input.Pengembalian
	retur.Status = "Disetujui"
	if err := db.Save(&retur).Error; err != nil {
		handleError(w, http.StatusInternalServerError, "Failed to update return")
		return
	}
	respondJSON(w, http.StatusOK, retur)
}

func disapproveReturHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id, err := strconv.Atoi(vars["id"])
	if err != nil {
		handleError(w, http.StatusBadRequest, "Invalid ID format")
		return
	}

	var retur Retur
	if err := db.First(&retur, id).Error; err != nil {
		handleError(w, http.StatusNotFound, "Return not found")
		return
	}

	retur.Status = "Tidak Disetujui"
	if err := db.Save(&retur).Error; err != nil {
		handleError(w, http.StatusInternalServerError, "Failed to update return")
		return
	}
	respondJSON(w, http.StatusOK, retur)
}

func deleteReturHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id, err := strconv.Atoi(vars["id"])
	if err != nil {
		handleError(w, http.StatusBadRequest, "Invalid ID format")
		return
	}

	var retur Retur
	if err := db.First(&retur, id).Error; err != nil {
		handleError(w, http.StatusNotFound, "Return not found")
		return
	}

	deletedIDs = append(deletedIDs, retur.ID)
	deletedStack.Push(retur)
	if err := db.Delete(&retur).Error; err != nil {
		handleError(w, http.StatusInternalServerError, "Failed to delete return")
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"message": fmt.Sprintf("Return with ID %d deleted", id)})
}

func undoDeleteReturHandler(w http.ResponseWriter, r *http.Request) {
	if deletedStack.IsEmpty() {
		handleError(w, http.StatusBadRequest, "No returns to undo")
		return
	}

	item, _ := deletedStack.Pop()
	if err := db.Create(&item).Error; err != nil {
		handleError(w, http.StatusInternalServerError, "Failed to restore return")
		return
	}
	respondJSON(w, http.StatusOK, item)
}

func main() {
	initDB()

	r := mux.NewRouter()
	r.HandleFunc("/retur", getReturs).Methods("GET")
	r.HandleFunc("/retur", createRetur).Methods("POST")
	r.HandleFunc("/retur/{id}/approve", approveReturHandler).Methods("POST")
	r.HandleFunc("/retur/{id}/disapprove", disapproveReturHandler).Methods("POST")
	r.HandleFunc("/retur/{id}/delete", deleteReturHandler).Methods("DELETE")
	r.HandleFunc("/retur/undo", undoDeleteReturHandler).Methods("POST")

	http.ListenAndServe(":8080", r)
}