package main

import (
	"encoding/json" // Untuk encoding dan decoding data JSON
	"fmt"           // Untuk mencetak output ke console
	"net/http"      // Untuk membuat server HTTP
	"strconv"       // Untuk konversi string ke angka dan sebaliknya

	"github.com/gorilla/mux" // Library untuk routing HTTP
	"gorm.io/driver/mysql"  // Driver MySQL untuk GORM
	"gorm.io/gorm"          // Library ORM untuk pengelolaan database
)

// Struct Retur untuk merepresentasikan data retur yang ada di database
type Retur struct {
	ID          int    `json:"id"`           // ID unik untuk retur
	Barang      string `json:"barang"`       // Nama barang yang diretur
	Alasan      string `json:"alasan"`       // Alasan retur
	Status      string `json:"status"`       // Status retur (Dalam Proses, Disetujui, dll.)
	Pengembalian string `json:"pengembalian"` // Jenis pengembalian (barang atau uang)
}

// Struct Stack adalah implementasi stack generik dengan tipe data generik (T)
type Stack[T any] struct {
	items []T // Slice untuk menyimpan elemen stack
}

// Method Push untuk menambahkan elemen ke stack
func (s *Stack[T]) Push(item T) {
	s.items = append(s.items, item) // Menambahkan elemen ke slice
}

// Method Pop untuk mengambil elemen terakhir dari stack
func (s *Stack[T]) Pop() (T, bool) {
	if len(s.items) == 0 { // Jika stack kosong
		var zero T
		return zero, false // Kembalikan nilai default tipe T dan false
	}
	item := s.items[len(s.items)-1]       // Ambil elemen terakhir
	s.items = s.items[:len(s.items)-1]    // Hapus elemen terakhir dari slice
	return item, true                     // Kembalikan elemen dan true
}

// Method IsEmpty untuk mengecek apakah stack kosong
func (s *Stack[T]) IsEmpty() bool {
	return len(s.items) == 0 // Return true jika slice kosong
}

var (
	db           *gorm.DB       // Variabel global untuk koneksi database
	deletedStack Stack[Retur]   // Stack untuk menyimpan data retur yang dihapus
	deletedIDs   []int          // Slice untuk menyimpan ID barang yang dihapus
)

// Function initDB untuk menginisialisasi koneksi ke database
func initDB() {
	var err error
	dsn := "root:@tcp(127.0.0.1:3306)/retur_db?charset=utf8mb4&parseTime=True&loc=Local" // Data Source Name untuk MySQL
	db, err = gorm.Open(mysql.Open(dsn), &gorm.Config{}) // Membuka koneksi database
	if err != nil {
		panic("Failed to connect to database: " + err.Error()) // Berhenti jika koneksi gagal
	}
	db.AutoMigrate(&Retur{}) // Membuat tabel secara otomatis berdasarkan struct Retur
}

// Function respondJSON untuk mengirimkan respon dalam format JSON
func respondJSON(w http.ResponseWriter, status int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json") // Set header respon sebagai JSON
	w.WriteHeader(status)                              // Set status HTTP
	json.NewEncoder(w).Encode(payload)                // Encode payload ke JSON dan kirim ke client
}

// Function handleError untuk mengirimkan pesan error ke client
func handleError(w http.ResponseWriter, status int, message string) {
	respondJSON(w, status, map[string]string{"error": message}) // Kirim pesan error dalam format JSON
}

// Function getReturs untuk mendapatkan semua data retur dari database
func getReturs(w http.ResponseWriter, r *http.Request) {
	var returs []Retur
	if err := db.Find(&returs).Error; err != nil { // Ambil semua data retur dari database
		handleError(w, http.StatusInternalServerError, "Failed to retrieve returns") // Kirim error jika gagal
		return
	}
	respondJSON(w, http.StatusOK, returs) // Kirim data retur ke client dalam format JSON
}

// Function createRetur untuk menambahkan data retur baru
func createRetur(w http.ResponseWriter, r *http.Request) {
	var newRetur Retur
	if err := json.NewDecoder(r.Body).Decode(&newRetur); err != nil { // Decode body request ke struct Retur
		handleError(w, http.StatusBadRequest, "Invalid input") // Kirim error jika format input salah
		return
	}

	if len(deletedIDs) > 0 { // Jika ada ID yang sebelumnya dihapus
		newRetur.ID = deletedIDs[len(deletedIDs)-1]      // Gunakan kembali ID yang dihapus
		deletedIDs = deletedIDs[:len(deletedIDs)-1]      // Hapus ID tersebut dari slice
	} else {
		var lastRetur Retur
		if err := db.Order("id desc").First(&lastRetur).Error; err == nil { // Ambil ID retur terakhir
			newRetur.ID = lastRetur.ID + 1 // Tambahkan ID baru
		} else {
			newRetur.ID = 1 // Jika belum ada data, mulai dari ID 1
		}
	}

	newRetur.Status = "Dalam Proses" // Set status awal retur
	if err := db.Create(&newRetur).Error; err != nil { // Simpan data retur baru ke database
		handleError(w, http.StatusInternalServerError, "Failed to create return") // Kirim error jika gagal
		return
	}
	respondJSON(w, http.StatusCreated, newRetur) // Kirim data retur baru ke client
}

// Function approveReturHandler untuk menyetujui retur
func approveReturHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)                          // Ambil parameter dari URL
	id, err := strconv.Atoi(vars["id"])          // Konversi ID dari string ke int
	if err != nil {
		handleError(w, http.StatusBadRequest, "Invalid ID format") // Kirim error jika format ID salah
		return
	}

	var input struct {
		Pengembalian string `json:"pengembalian"` // Jenis pengembalian (barang atau uang)
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil { // Decode body request
		handleError(w, http.StatusBadRequest, "Invalid input") // Kirim error jika input salah
		return
	}

	if input.Pengembalian != "barang" && input.Pengembalian != "uang" { // Validasi input pengembalian
		handleError(w, http.StatusBadRequest, "Pengembalian must be 'barang' or 'uang'")
		return
	}

	var retur Retur
	if err := db.First(&retur, id).Error; err != nil { // Cari retur berdasarkan ID
		handleError(w, http.StatusNotFound, "Return not found") // Kirim error jika retur tidak ditemukan
		return
	}

	retur.Pengembalian = input.Pengembalian // Set jenis pengembalian
	retur.Status = "Disetujui"              // Ubah status menjadi disetujui
	if err := db.Save(&retur).Error; err != nil { // Simpan perubahan ke database
		handleError(w, http.StatusInternalServerError, "Failed to update return") // Kirim error jika gagal
		return
	}
	respondJSON(w, http.StatusOK, retur) // Kirim data retur yang diperbarui ke client
}

// Function disapproveReturHandler untuk menolak retur
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

	retur.Status = "Tidak Disetujui" // Set status menjadi tidak disetujui
	if err := db.Save(&retur).Error; err != nil {
		handleError(w, http.StatusInternalServerError, "Failed to update return")
		return
	}
	respondJSON(w, http.StatusOK, retur) // Kirim data retur yang diperbarui ke client
}

// Function deleteReturHandler untuk menghapus retur
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

	deletedIDs = append(deletedIDs, retur.ID) // Simpan ID retur yang dihapus
	deletedStack.Push(retur)                 // Simpan data retur yang dihapus ke stack
	if err := db.Delete(&retur).Error; err != nil { // Hapus data retur dari database
		handleError(w, http.StatusInternalServerError, "Failed to delete return")
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"message": fmt.Sprintf("Return with ID %d deleted", id)})
}

// Function undoDeleteReturHandler untuk membatalkan penghapusan retur
func undoDeleteReturHandler(w http.ResponseWriter, r *http.Request) {
	if deletedStack.IsEmpty() { // Cek apakah stack kosong
		handleError(w, http.StatusBadRequest, "No returns to undo")
		return
	}

	item, _ := deletedStack.Pop() // Ambil data retur terakhir dari stack
	if err := db.Create(&item).Error; err != nil { // Simpan data retur kembali ke database
		handleError(w, http.StatusInternalServerError, "Failed to restore return")
		return
	}
	respondJSON(w, http.StatusOK, item) // Kirim data retur yang dipulihkan ke client
}

func main() {
	initDB() // Inisialisasi database

	r := mux.NewRouter() // Buat router baru
	// Rute untuk API
	r.HandleFunc("/retur", getReturs).Methods("GET")
	r.HandleFunc("/retur", createRetur).Methods("POST")
	r.HandleFunc("/retur/{id}/approve", approveReturHandler).Methods("POST")
	r.HandleFunc("/retur/{id}/disapprove", disapproveReturHandler).Methods("POST")
	r.HandleFunc("/retur/{id}/delete", deleteReturHandler).Methods("DELETE")
	r.HandleFunc("/retur/undo", undoDeleteReturHandler).Methods("POST")

	http.ListenAndServe(":8080", r) // Jalankan server pada port 8080
}
