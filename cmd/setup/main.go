package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"one_more_percent/internal/config"
	sheetrepo "one_more_percent/internal/repository/sheets"
)

func main() {
	seed := flag.Bool("seed", true, "isi jadwal contoh jika worksheet schedules masih kosong")
	flag.Parse()

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, "konfigurasi gagal:", err)
		os.Exit(1)
	}

	ctx := context.Background()
	client, err := sheetrepo.NewClient(ctx, cfg)
	if err != nil {
		fmt.Fprintln(os.Stderr, "inisialisasi sheets gagal:", err)
		os.Exit(1)
	}
	if err := client.EnsureSheets(ctx); err != nil {
		fmt.Fprintln(os.Stderr, "menyiapkan worksheet gagal:", err)
		os.Exit(1)
	}
	fmt.Println("Worksheet dan header siap dipakai.")

	if !*seed {
		return
	}

	repo := sheetrepo.NewScheduleRepository(client, cfg.SheetsCacheTTL)
	count, err := sheetrepo.SeedDefaultSchedules(ctx, client, repo)
	if err != nil {
		fmt.Fprintln(os.Stderr, "seed gagal:", err)
		os.Exit(1)
	}
	if count == 0 {
		fmt.Println("Seed dilewati: worksheet schedules sudah berisi data.")
		return
	}
	fmt.Printf("Seed selesai: %d jadwal ditambahkan.\n", count)
}
