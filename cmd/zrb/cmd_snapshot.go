package main

func runSnapshotCommand(pool, dataset, prefix string) error {
	return createSnapshot(pool, dataset, prefix)
}
