#!/usr/bin/env python3
"""
Analyze results from Zyzzyva experiments.

This script processes the JSON statistics files generated during experiments
and produces summary graphs and tables.
"""

import argparse
import json
import os
import sys
import glob
from pathlib import Path
import matplotlib.pyplot as plt
import numpy as np
import pandas as pd
from tabulate import tabulate

def load_experiment_config(experiment_dir):
    """Load experiment configuration"""
    config_path = os.path.join(experiment_dir, 'config.json')
    if not os.path.exists(config_path):
        print(f"Error: No config.json found in {experiment_dir}")
        return None
    
    with open(config_path, 'r') as f:
        return json.load(f)

def load_client_stats(experiment_dir):
    """Load all client statistics JSON files from the experiment"""
    stats_dir = os.path.join(experiment_dir, 'stats')
    if not os.path.exists(stats_dir):
        stats_dir = experiment_dir  # Try the main directory
    
    client_stats = []
    for stats_file in glob.glob(os.path.join(stats_dir, 'client_*_stats.json')):
        try:
            with open(stats_file, 'r') as f:
                stats = json.load(f)
                # Extract client ID from filename
                filename = os.path.basename(stats_file)
                client_id = int(filename.split('_')[1])
                stats['client_id'] = client_id
                client_stats.append(stats)
        except Exception as e:
            print(f"Error loading {stats_file}: {e}")
    
    return client_stats

def analyze_experiment(experiment_dir):
    """Analyze a single experiment"""
    print(f"Analyzing experiment: {experiment_dir}")
    
    # Load experiment configuration
    config = load_experiment_config(experiment_dir)
    if not config:
        return None
    
    # Load client statistics
    client_stats = load_client_stats(experiment_dir)
    if not client_stats:
        print(f"No client statistics found in {experiment_dir}")
        return None
    
    # Calculate aggregate statistics
    total_requests = sum(stats.get('total_requests', 0) for stats in client_stats)
    successful_requests = sum(stats.get('successful_requests', 0) for stats in client_stats)
    fast_path = sum(stats.get('fast_path_completed', 0) for stats in client_stats)
    slow_path = sum(stats.get('slow_path_completed', 0) for stats in client_stats)
    
    # Calculate average latencies
    avg_latencies = [stats.get('avg_response_time_ms', 0) for stats in client_stats]
    avg_latency = np.mean(avg_latencies) if avg_latencies else 0
    
    # Calculate throughput
    throughputs = [stats.get('throughput_req_sec', 0) for stats in client_stats]
    total_throughput = sum(throughputs)
    
    # Create summary
    summary = {
        'experiment_dir': experiment_dir,
        'config': config,
        'total_requests': total_requests,
        'successful_requests': successful_requests,
        'success_rate': (successful_requests / total_requests * 100) if total_requests > 0 else 0,
        'fast_path_rate': (fast_path / successful_requests * 100) if successful_requests > 0 else 0,
        'avg_latency_ms': avg_latency,
        'throughput_req_sec': total_throughput,
        'client_stats': client_stats
    }
    
    return summary

def print_summary_table(summaries):
    """Print a summary table of all experiments"""
    if not summaries:
        print("No experiment summaries to display.")
        return
    
    # Prepare table data
    table_data = []
    for summary in summaries:
        config = summary['config']
        row = [
            os.path.basename(summary['experiment_dir']),
            config.get('f', 'N/A'),
            config.get('n', 'N/A'),
            config.get('clients', 'N/A'),
            config.get('batch_size', 'N/A'),
            summary['total_requests'],
            f"{summary['success_rate']:.1f}%",
            f"{summary['fast_path_rate']:.1f}%",
            f"{summary['avg_latency_ms']:.2f}",
            f"{summary['throughput_req_sec']:.1f}"
        ]
        table_data.append(row)
    
    # Print table
    headers = ['Experiment', 'F', 'N', 'Clients', 'Batch', 'Requests', 
              'Success%', 'FastPath%', 'Avg Latency(ms)', 'Throughput(req/s)']
    print("\nExperiment Summary:")
    print(tabulate(table_data, headers=headers, tablefmt='grid'))

def plot_throughput_vs_latency(summaries, output_dir):
    """Create a throughput vs latency plot"""
    if not summaries:
        return
    
    plt.figure(figsize=(10, 6))
    
    # Group by batch size
    batch_sizes = sorted(set(s['config'].get('batch_size', 1) for s in summaries))
    markers = ['o', 's', '^', 'x', 'D', '*', 'v', 'p']
    
    for i, batch_size in enumerate(batch_sizes):
        batch_summaries = [s for s in summaries if s['config'].get('batch_size', 1) == batch_size]
        
        x = [s['throughput_req_sec'] for s in batch_summaries]
        y = [s['avg_latency_ms'] for s in batch_summaries]
        
        marker = markers[i % len(markers)]
        plt.scatter(x, y, label=f'Batch Size {batch_size}', marker=marker, s=100)
    
    plt.xlabel('Throughput (requests/second)')
    plt.ylabel('Average Latency (ms)')
    plt.title('Throughput vs Latency')
    plt.grid(True)
    plt.legend()
    
    # Save plot
    output_path = os.path.join(output_dir, 'throughput_vs_latency.png')
    plt.savefig(output_path)
    print(f"Saved plot to {output_path}")
    
    plt.close()

def plot_batch_size_impact(summaries, output_dir):
    """Create plots showing the impact of batch size"""
    if not summaries:
        return
    
    # Filter summaries with the same configuration except batch size
    batch_sizes = sorted(set(s['config'].get('batch_size', 1) for s in summaries))
    
    # Create DataFrame for analysis
    data = []
    for s in summaries:
        data.append({
            'batch_size': s['config'].get('batch_size', 1),
            'f': s['config'].get('f', 1),
            'clients': s['config'].get('clients', 1),
            'throughput': s['throughput_req_sec'],
            'latency': s['avg_latency_ms'],
            'fast_path_rate': s['fast_path_rate']
        })
    
    df = pd.DataFrame(data)
    
    # Plot throughput vs batch size
    plt.figure(figsize=(10, 6))
    
    # Group by f (fault tolerance)
    f_values = sorted(df['f'].unique())
    markers = ['o', 's', '^', 'x', 'D']
    
    for i, f in enumerate(f_values):
        f_data = df[df['f'] == f]
        
        # Group by number of clients
        client_values = sorted(f_data['clients'].unique())
        
        for j, clients in enumerate(client_values):
            client_data = f_data[f_data['clients'] == clients]
            
            # Sort by batch size
            client_data = client_data.sort_values('batch_size')
            
            marker = markers[(i*len(client_values) + j) % len(markers)]
            plt.plot(client_data['batch_size'], client_data['throughput'], 
                     marker=marker, label=f'F={f}, Clients={clients}',
                     linewidth=2, markersize=8)
    
    plt.xlabel('Batch Size')
    plt.ylabel('Throughput (requests/second)')
    plt.title('Impact of Batch Size on Throughput')
    plt.grid(True)
    plt.legend()
    
    # Save plot
    output_path = os.path.join(output_dir, 'batch_size_throughput.png')
    plt.savefig(output_path)
    print(f"Saved plot to {output_path}")
    
    plt.close()

def main():
    parser = argparse.ArgumentParser(description='Analyze Zyzzyva experiment results')
    parser.add_argument('experiment_dirs', nargs='+', help='Experiment directories to analyze')
    parser.add_argument('--output', '-o', default='analysis_results', help='Output directory for plots')
    
    args = parser.parse_args()
    
    # Create output directory
    os.makedirs(args.output, exist_ok=True)
    
    # Analyze each experiment
    summaries = []
    for exp_dir in args.experiment_dirs:
        summary = analyze_experiment(exp_dir)
        if summary:
            summaries.append(summary)
    
    if not summaries:
        print("No valid experiment results found.")
        sys.exit(1)
    
    # Print summary table
    print_summary_table(summaries)
    
    # Generate plots
    plot_throughput_vs_latency(summaries, args.output)
    plot_batch_size_impact(summaries, args.output)
    
    print(f"\nAnalysis complete. Results saved to {args.output}/")

if __name__ == "__main__":
    main()