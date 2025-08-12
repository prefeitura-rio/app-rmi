import pandas as pd
import matplotlib.pyplot as plt
import seaborn as sns
from datetime import datetime
import argparse
import os

def plot_usage(csv_path):
    # Read the CSV file
    df = pd.read_csv(csv_path)

    # Convert Time column to datetime
    df['Time'] = pd.to_datetime(df['Time'])

    # Convert CPU_m to numeric (removing 'm' suffix)
    df['CPU_m'] = df['CPU_m'].str.replace('m', '').astype(float)

    # Convert Memory to numeric (removing 'Mi' suffix)
    df['Memory'] = df['Memory'].str.replace('Mi', '').astype(float)

    # Create a figure with 3 subplots
    fig, (ax1, ax2, ax3) = plt.subplots(3, 1, figsize=(15, 12))
    fig.suptitle('RMI Application Resource Usage', fontsize=16)

    # Plot RPS
    ax1.plot(df['Time'], df['Istio_RPS'], 'b-', label='Requests Per Second')
    ax1.set_title('Requests Per Second (RPS)')
    ax1.set_ylabel('RPS')
    ax1.grid(True)
    ax1.legend()

    # Plot CPU
    ax2.plot(df['Time'], df['CPU_m'], 'r-', label='CPU Usage')
    ax2.set_title('CPU Usage')
    ax2.set_ylabel('CPU (millicores)')
    ax2.grid(True)
    ax2.legend()

    # Plot Memory
    ax3.plot(df['Time'], df['Memory'], 'g-', label='Memory Usage')
    ax3.set_title('Memory Usage')
    ax3.set_ylabel('Memory (Mi)')
    ax3.set_xlabel('Time')
    ax3.grid(True)
    ax3.legend()

    # Rotate x-axis labels for better readability
    for ax in [ax1, ax2, ax3]:
        plt.setp(ax.get_xticklabels(), rotation=45)

    # Adjust layout to prevent label cutoff
    plt.tight_layout()

    # Generate output filename based on input filename
    base_name = os.path.splitext(os.path.basename(csv_path))[0]
    output_file = f"{base_name}_plot.png"

    # Save the plot
    plt.savefig(output_file, dpi=300, bbox_inches='tight')
    print(f"\nPlot saved as: {output_file}")

    # Calculate some statistics
    print("\nResource Usage Statistics:")
    print(f"RPS - Max: {df['Istio_RPS'].max():.2f}, Min: {df['Istio_RPS'].min():.2f}, Avg: {df['Istio_RPS'].mean():.2f}")
    print(f"CPU - Max: {df['CPU_m'].max():.2f}m, Min: {df['CPU_m'].min():.2f}m, Avg: {df['CPU_m'].mean():.2f}m")
    print(f"Memory - Max: {df['Memory'].max():.2f}Mi, Min: {df['Memory'].min():.2f}Mi, Avg: {df['Memory'].mean():.2f}Mi")

    # Calculate correlation between RPS and CPU
    correlation = df['Istio_RPS'].corr(df['CPU_m'])
    print(f"\nCorrelation between RPS and CPU: {correlation:.2f}")

def main():
    parser = argparse.ArgumentParser(description='Plot RMI application resource usage from CSV data')
    parser.add_argument('csv_file', help='Path to the CSV file containing usage data')
    args = parser.parse_args()

    if not os.path.exists(args.csv_file):
        print(f"Error: File '{args.csv_file}' not found")
        return

    plot_usage(args.csv_file)

if __name__ == "__main__":
    main() 