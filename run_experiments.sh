#!/bin/bash

for i in {1..30}
do
  echo "--- Starting Experiment $i ---"
  if [ -f "experiment_${i}_snapshot.gob" ]; then
    echo "--- Continuing Experiment $i from snapshot ---"
    go run . -load "experiment_${i}_snapshot.gob" -snapshot "experiment_${i}_run_2_snapshot.gob" -entropy "experiment_${i}_run_2_entropies.csv"
  else
    go run . -snapshot "experiment_${i}_snapshot.gob" -entropy "experiment_${i}_entropies.csv"
  fi
  echo "--- Experiment $i finished ---"
done