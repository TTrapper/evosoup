
import pandas as pd
import plotly.graph_objects as go
import glob
import os

# Find all entropy CSV files
csv_files = sorted(glob.glob("experiment_*_entropies.csv"))

if not csv_files:
    print("No experiment CSV files found. Make sure you have run the experiments first.")
    exit()

fig = go.Figure()

# Process each CSV file
for i, file_path in enumerate(csv_files):
    try:
        # Extract experiment number from filename
        experiment_num = os.path.basename(file_path).split('_')[1]
        
        df = pd.read_csv(file_path)
        
        if 'Generation' in df.columns and 'Entropy' in df.columns:
            fig.add_trace(
                go.Scatter(
                    x=df['Generation'],
                    y=df['Entropy'],
                    mode='lines',
                    name=f'Experiment {experiment_num}'
                )
            )
        else:
            print(f"Warning: Skipping {file_path} - missing 'Generation' or 'Entropy' columns.")

    except Exception as e:
        print(f"Error processing {file_path}: {e}")


# Update layout
fig.update_layout(
    title_text="Entropy Over Generations for All Experiments",
    xaxis_title="Generation",
    yaxis_title="Soup Entropy",
    legend_title="Experiment",
    hovermode="x unified"
)

# Save to HTML and show
output_filename = "entropy_plots.html"
fig.write_html(output_filename)

print(f"Plot saved to {output_filename}")
print("You can open this file in your web browser to view the interactive plot.")

