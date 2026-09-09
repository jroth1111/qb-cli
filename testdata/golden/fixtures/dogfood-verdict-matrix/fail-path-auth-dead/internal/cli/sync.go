package cli

func syncLive(store any) {
	store.UpsertLive()
}
