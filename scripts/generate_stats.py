#!/usr/bin/env python3
"""
Garage Trip Chores Statistics & Multi-Year Comparison Generator
Generates statistical reports, metrics, visual charts, interactive HTML dashboards,
and an attendee-facing PDF report comparing GT 6.9 (2025) and GT 7 (2026).
"""

import argparse
import base64
import json
import os
import shutil
import sqlite3
import subprocess
import sys

import matplotlib
matplotlib.use("Agg")
import matplotlib.pyplot as plt
import matplotlib.patches as patches
import matplotlib.ticker as mtick
import numpy as np
import pandas as pd
from scipy import stats

def parse_args():
    parser = argparse.ArgumentParser(description="Generate Chores Statistics & Multi-Year Comparisons")
    default_db26 = "data-prod/gt7-2026.sqlite" if os.path.exists("data-prod/gt7-2026.sqlite") else ("data/gt7.sqlite" if os.path.exists("data/gt7.sqlite") else "data/db.sqlite")
    default_db25 = "data-prod/gt6.9-2025.sqlite"
    
    parser.add_argument("--db", "--db-2026", dest="db_2026", default=default_db26, help="Path to 2026 SQLite database")
    parser.add_argument("--db-2025", default=default_db25, help="Path to 2025 SQLite database")
    parser.add_argument("--user-map", default="data/discord_user_map.json", help="Path to Discord user mapping JSON")
    parser.add_argument("--output-dir", default="output", help="Directory to save charts and reports")
    parser.add_argument("--sample-period-2026", type=int, default=10, help="Presence tracker sample period in minutes for 2026")
    parser.add_argument("--sample-period-2025", type=int, default=2, help="Presence tracker sample period in minutes for 2025")
    parser.add_argument("--no-pdf", action="store_true", help="Skip PDF report generation")
    return parser.parse_args()

def load_user_map(map_path):
    if os.path.exists(map_path):
        with open(map_path, "r", encoding="utf-8") as f:
            return json.load(f)
    return {}

def format_user_label(uid, user_map):
    if uid == "1497140782900187146":
        return "@Klára Vonšovská"
    if uid == "1415037315130331247":
        return "@Rasťo (Rasťo)"
    if uid == "254931468386566144":
        return "@Ferryq (Fanda)"
    
    if uid not in user_map:
        return f"User {uid[-4:]}"
    
    info = user_map[uid]
    nick = info.get("nick")
    uname = info.get("username", "")
    gname = info.get("global_name", "")
    
    if nick:
        if nick.startswith(f"{uname} ("):
            return f"@{nick}"
        if "(" in nick and ")" in nick:
            return f"@{nick}"
        return f"@{uname} ({nick})"
    elif gname and gname != uname:
        return f"@{uname} ({gname})"
    elif uname:
        return f"@{uname}"
    return f"User {uid[-4:]}"

def get_short_name(uid, user_map):
    if uid == "1497140782900187146":
        return "Klára V."
    if uid == "1415037315130331247":
        return "Rasťo"
    if uid == "254931468386566144":
        return "Fanda"
    if uid not in user_map:
        return f"User {uid[-4:]}"
    info = user_map[uid]
    nick = info.get("nick")
    if nick:
        # e.g. "Fisa (Zbyněk Fišer)" -> "Fisa"
        if "(" in nick:
            return nick.split("(")[0].strip()
        return nick
    return info.get("username", f"User {uid[-4:]}")

def fetch_year_data(con, sample_period_min, user_map, exclude_uids=None, max_presence_ts=None):
    if exclude_uids is None:
        exclude_uids = set()
    
    # Presence query
    if max_presence_ts:
        presence_query = f"""
            SELECT user_id, count(*) as presence_ticks
            FROM presence_logs
            WHERE timestamp <= '{max_presence_ts}'
            GROUP BY user_id
        """
    else:
        presence_query = """
            SELECT user_id, count(*) as presence_ticks
            FROM presence_logs
            GROUP BY user_id
        """
    df_presence = pd.read_sql_query(presence_query, con)

    # Work logs query
    df_work = pd.read_sql_query("""
        SELECT 
            wl.user_id,
            sum(wl.time_spent_min) as worked_min,
            count(*) as chores_done,
            sum(case when wl.self_reported = 1 then 1 else 0 end) as self_reported_count,
            sum(case when wl.self_reported = 1 then wl.time_spent_min else 0 end) as self_reported_min
        FROM work_logs wl
        JOIN chores c ON c.id = wl.chore_id
        WHERE c.cancelled IS NULL
        GROUP BY wl.user_id
    """, con)

    # Assignments query
    df_assign = pd.read_sql_query("""
        SELECT 
            user_id,
            count(*) as total_assignments,
            sum(case when acked is not null then 1 else 0 end) as acked_count,
            sum(case when refused is not null then 1 else 0 end) as refused_count,
            sum(case when timeouted is not null then 1 else 0 end) as timeouted_count
        FROM chore_assignments
        GROUP BY user_id
    """, con)

    # Merge
    df_users = pd.merge(df_presence, df_work, on="user_id", how="outer").fillna(0)
    df_users = pd.merge(df_users, df_assign, on="user_id", how="outer").fillna(0)

    # Filter excluded
    df_users = df_users[~df_users["user_id"].isin(exclude_uids)].reset_index(drop=True)

    df_users["presence_min"] = df_users["presence_ticks"] * sample_period_min
    df_users["presence_hours"] = df_users["presence_min"] / 60.0
    df_users["worked_hours"] = df_users["worked_min"] / 60.0
    df_users["pct_working"] = np.where(
        df_users["presence_min"] > 0,
        (df_users["worked_min"] / df_users["presence_min"]) * 100.0,
        0.0
    )
    df_users["label"] = df_users["user_id"].apply(lambda uid: format_user_label(uid, user_map))
    df_users["short_name"] = df_users["user_id"].apply(lambda uid: get_short_name(uid, user_map))
    df_users = df_users.sort_values(by="pct_working", ascending=False).reset_index(drop=True)

    # Daily activity
    df_daily = pd.read_sql_query("""
        SELECT 
            substr(c.completed, 1, 10) as date,
            count(distinct c.id) as completed_chores,
            count(wl.id) as work_sessions,
            sum(wl.time_spent_min) as total_min
        FROM chores c
        JOIN work_logs wl ON wl.chore_id = c.id
        WHERE c.completed IS NOT NULL AND c.cancelled IS NULL
        GROUP BY date
        ORDER BY date
    """, con)

    # Top chores
    df_top_chores = pd.read_sql_query("""
        SELECT 
            c.name,
            sum(wl.time_spent_min) as total_min,
            count(distinct wl.id) as sessions,
            count(distinct wl.user_id) as workers,
            avg(wl.time_spent_min) as avg_min_per_session
        FROM chores c
        JOIN work_logs wl ON wl.chore_id = c.id
        WHERE c.cancelled IS NULL
        GROUP BY c.name
        ORDER BY total_min DESC
    """, con)

    return df_users, df_daily, df_top_chores

def plot_pct_spent_working(df, mean_pct, std_pct, output_file):
    """
    Recreates the iconic single-year bar chart with ±1 SD bounding box.
    """
    plt.style.use("default")
    fig, ax = plt.subplots(figsize=(15, 8.5), dpi=300)
    fig.patch.set_facecolor("white")
    ax.set_facecolor("white")

    x = np.arange(len(df))
    bars = ax.bar(x, df["pct_working"], width=0.58, color="#4285F4", label="pct spent working", zorder=3)
    avg_line = ax.axhline(mean_pct, color="#EA4335", linewidth=2.2, label="avg", zorder=4)

    max_val = max(df["pct_working"].max() * 1.15, 3.2)
    ax.set_ylim(0, max_val)
    ax.yaxis.set_major_formatter(mtick.PercentFormatter(decimals=2))
    ax.grid(axis="y", color="#E0E0E0", linestyle="-", linewidth=1, zorder=1)
    ax.set_axisbelow(True)

    for s in ["top", "right", "left", "bottom"]:
        ax.spines[s].set_color("#CCCCCC")

    ax.set_xticks(x)
    ax.set_xticklabels(df["label"], rotation=50, ha="right", fontsize=9.5)

    for bar in bars:
        h = bar.get_height()
        ax.text(
            bar.get_x() + bar.get_width() / 2.0,
            h + 0.04,
            f"{h:.2f}%",
            ha="center",
            va="bottom",
            fontsize=8.5,
            color="#3367D6",
            fontweight="bold",
            bbox=dict(boxstyle="round,pad=0.15", facecolor="white", edgecolor="none", alpha=0.85),
            zorder=6
        )

    in_sd_mask = (df["pct_working"] >= (mean_pct - std_pct)) & (df["pct_working"] <= (mean_pct + std_pct))
    sd_indices = np.where(in_sd_mask)[0]

    if len(sd_indices) > 0:
        first_idx = sd_indices[0]
        last_idx = sd_indices[-1]
        rect_x = first_idx - 0.45
        rect_width = (last_idx - first_idx) + 0.90
        rect_y_bottom = max(0.0, mean_pct - std_pct)
        rect_y_top = mean_pct + std_pct + 0.12
        rect_height = rect_y_top - rect_y_bottom

        rect = patches.Rectangle(
            (rect_x, rect_y_bottom),
            rect_width,
            rect_height,
            linewidth=2.5,
            edgecolor="#EA4335",
            facecolor="none",
            linestyle="-",
            zorder=5,
            label=f"±1 SD range ({len(sd_indices)}/{len(df)} people)"
        )
        ax.add_patch(rect)

    legend = ax.legend(
        loc="upper center",
        bbox_to_anchor=(0.5, 1.08),
        ncol=3,
        frameon=False,
        fontsize=11
    )
    for handle in legend.legend_handles:
        handle.set_alpha(1.0)

    plt.tight_layout()
    plt.savefig(output_file, dpi=300)
    plt.close()
    print(f"Saved: {output_file}")

def plot_distribution(df, mean_pct, std_pct, output_file):
    """
    2-panel distribution visualization: Histogram & Normal Fit.
    """
    plt.style.use("default")
    fig, (ax_top, ax_bottom) = plt.subplots(2, 1, figsize=(10, 11), dpi=300)
    fig.patch.set_facecolor("white")

    # Top: Histogram
    ax_top.set_facecolor("white")
    bins = np.arange(0.0, max(df["pct_working"].max() + 0.5, 3.5), 0.35)
    counts, edges, _ = ax_top.hist(
        df["pct_working"],
        bins=bins,
        color="#4285F4",
        edgecolor="white",
        linewidth=1.5,
        zorder=3
    )

    ax_top.set_title("pct spent working", fontsize=16, color="#444444", pad=15, loc="left", fontweight="500")
    ax_top.set_ylabel("Count of People", fontsize=11, color="#555555")
    ax_top.grid(axis="y", color="#E0E0E0", linestyle="-", linewidth=1, zorder=1)
    ax_top.set_axisbelow(True)
    ax_top.xaxis.set_major_formatter(mtick.PercentFormatter(decimals=1))
    
    for s in ["top", "right"]: ax_top.spines[s].set_visible(False)
    for s in ["left", "bottom"]: ax_top.spines[s].set_color("#CCCCCC")

    for count, edge in zip(counts, edges):
        if count > 0:
            ax_top.text(
                edge + (edges[1] - edges[0]) / 2.0,
                count + 0.15,
                f"{int(count)}",
                ha="center",
                va="bottom",
                fontsize=10,
                fontweight="bold",
                color="#3367D6"
            )

    # Bottom: Gaussian curve
    ax_bottom.set_facecolor("white")
    x_curve = np.linspace(-1.0, 4.5, 500)
    y_pdf = stats.norm.pdf(x_curve, mean_pct, std_pct)

    ax_bottom.plot(x_curve, y_pdf, color="#4285F4", linewidth=2.5, label=f"Normal Fit (μ={mean_pct:.2f}%, σ={std_pct:.2f}%)", zorder=3)
    ax_bottom.fill_between(x_curve, 0, y_pdf, color="#4285F4", alpha=0.15, zorder=2)
    ax_bottom.axvline(0.0, color="#222222", linewidth=1.2, linestyle="-", zorder=4)
    ax_bottom.axvline(mean_pct, color="#EA4335", linewidth=1.8, linestyle="--", label=f"Mean ({mean_pct:.2f}%)", zorder=4)

    x_sd = np.linspace(max(0, mean_pct - std_pct), mean_pct + std_pct, 200)
    y_sd = stats.norm.pdf(x_sd, mean_pct, std_pct)
    ax_bottom.fill_between(x_sd, 0, y_sd, color="#34A853", alpha=0.25, label=f"±1 SD [{max(0, mean_pct - std_pct):.2f}% - {mean_pct + std_pct:.2f}%]", zorder=2)

    ax_bottom.set_xlim(-1.0, 4.5)
    ax_bottom.set_ylim(0, max(y_pdf) * 1.15)
    ax_bottom.grid(True, color="#E0E0E0", linestyle="-", linewidth=1, zorder=1)
    ax_bottom.set_axisbelow(True)
    ax_bottom.xaxis.set_major_formatter(mtick.PercentFormatter(decimals=1))
    ax_bottom.set_xlabel("pct spent working", fontsize=11, color="#555555")
    ax_bottom.set_ylabel("Probability Density", fontsize=11, color="#555555")

    for s in ["top", "right"]: ax_bottom.spines[s].set_visible(False)
    for s in ["left", "bottom"]: ax_bottom.spines[s].set_color("#CCCCCC")

    ax_bottom.legend(loc="upper right", frameon=True, facecolor="white", edgecolor="#E0E0E0", fontsize=9.5)

    plt.tight_layout()
    plt.savefig(output_file, dpi=300)
    plt.close()
    print(f"Saved: {output_file}")

def plot_comprehensive_dashboard(df_users, df_daily, df_top_chores, mean_pct, std_pct, output_file):
    """
    Generates a 4-panel comprehensive infographic dashboard.
    """
    plt.style.use("default")
    fig, axes = plt.subplots(2, 2, figsize=(18, 14), dpi=300)
    fig.patch.set_facecolor("#F8F9FA")

    # Panel 1: Top Contributors
    ax1 = axes[0, 0]
    ax1.set_facecolor("white")
    df_sorted_min = df_users.sort_values(by="worked_min", ascending=True)
    y1 = np.arange(len(df_sorted_min))
    bars1 = ax1.barh(y1, df_sorted_min["worked_min"], color="#34A853", height=0.65, zorder=3)
    ax1.set_yticks(y1)
    ax1.set_yticklabels(df_sorted_min["label"], fontsize=8.5)
    ax1.set_xlabel("Total Worked Minutes", fontsize=10, fontweight="600")
    ax1.set_title("Total Work Contribution by User (Minutes)", fontsize=12, fontweight="bold", pad=10)
    ax1.grid(axis="x", color="#EEEEEE", linestyle="-", zorder=1)
    for bar in bars1:
        w = bar.get_width()
        ax1.text(w + 3, bar.get_y() + bar.get_height()/2.0, f"{int(w)}m", va="center", fontsize=8, color="#222222")

    # Panel 2: Response Breakdown
    ax2 = axes[0, 1]
    ax2.set_facecolor("white")
    df_assign_sorted = df_users.sort_values(by="total_assignments", ascending=True)
    y2 = np.arange(len(df_assign_sorted))
    ax2.barh(y2, df_assign_sorted["acked_count"], color="#4285F4", label="Acked (Accepted)", height=0.65, zorder=3)
    ax2.barh(y2, df_assign_sorted["timeouted_count"], left=df_assign_sorted["acked_count"], color="#FBBC05", label="Timeouted", height=0.65, zorder=3)
    ax2.barh(y2, df_assign_sorted["refused_count"], left=df_assign_sorted["acked_count"] + df_assign_sorted["timeouted_count"], color="#EA4335", label="Refused", height=0.65, zorder=3)
    ax2.set_yticks(y2)
    ax2.set_yticklabels(df_assign_sorted["label"], fontsize=8.5)
    ax2.set_xlabel("Number of Assignments", fontsize=10, fontweight="600")
    ax2.set_title("Chore Assignments: Response & Reliability Breakdown", fontsize=12, fontweight="bold", pad=10)
    ax2.grid(axis="x", color="#EEEEEE", linestyle="-", zorder=1)
    ax2.legend(loc="lower right", fontsize=9, frameon=True, facecolor="white", edgecolor="#E0E0E0")

    # Panel 3: Daily Timeline
    ax3 = axes[1, 0]
    ax3.set_facecolor("white")
    dates = df_daily["date"].tolist()
    chores_cnt = df_daily["completed_chores"].tolist()
    minutes_tot = df_daily["total_min"].tolist()

    x3 = np.arange(len(dates))
    bars3 = ax3.bar(x3, chores_cnt, color="#1A73E8", width=0.5, label="Completed Chores", zorder=3)
    ax3.set_xticks(x3)
    ax3.set_xticklabels([d[5:] for d in dates], fontsize=9)
    ax3.set_ylabel("Chores Completed", fontsize=10, fontweight="600", color="#1A73E8")
    ax3.grid(axis="y", color="#EEEEEE", linestyle="-", zorder=1)

    ax3_twin = ax3.twinx()
    line3 = ax3_twin.plot(x3, minutes_tot, color="#EA4335", marker="o", linewidth=2.5, label="Total Work Min", zorder=4)
    ax3_twin.set_ylabel("Total Work (Minutes)", fontsize=10, fontweight="600", color="#EA4335")
    ax3.set_title("Chore Activity Timeline Across the Week", fontsize=12, fontweight="bold", pad=10)

    # Panel 4: Top Chores
    ax4 = axes[1, 1]
    ax4.set_facecolor("white")
    top10 = df_top_chores.head(10).sort_values(by="total_min", ascending=True)
    y4 = np.arange(len(top10))
    short_names = [n if len(n) <= 34 else n[:31] + "..." for n in top10["name"]]
    bars4 = ax4.barh(y4, top10["total_min"], color="#9C27B0", height=0.6, zorder=3)
    ax4.set_yticks(y4)
    ax4.set_yticklabels(short_names, fontsize=8.5)
    ax4.set_xlabel("Total Work Minutes Spent", fontsize=10, fontweight="600")
    ax4.set_title("Top 10 Most Labor-Intensive Chores", fontsize=12, fontweight="bold", pad=10)
    ax4.grid(axis="x", color="#EEEEEE", linestyle="-", zorder=1)
    for bar, sessions in zip(bars4, top10["sessions"]):
        w = bar.get_width()
        ax4.text(w + 3, bar.get_y() + bar.get_height()/2.0, f"{int(w)}m ({sessions}x)", va="center", fontsize=8, color="#222222")

    for ax in [ax1, ax2, ax3, ax4, ax3_twin]:
        for s in ["top", "right"]: ax.spines[s].set_visible(False)
        for s in ["left", "bottom"]: ax.spines[s].set_color("#CCCCCC")

    plt.suptitle("Garage Trip 2026 - Comprehensive Chores & Fairness Dashboard", fontsize=17, fontweight="bold", y=0.98)
    plt.tight_layout(rect=[0, 0.03, 1, 0.96])
    plt.savefig(output_file, dpi=300)
    plt.close()
    print(f"Saved: {output_file}")

def plot_yoy_comparison(df_both, mean25, mean26, output_file):
    """
    Grouped horizontal bar chart comparing 2025 vs 2026 % spent working with delta badges.
    """
    plt.style.use("default")
    df_sorted = df_both.sort_values(by="combined_pct", ascending=True).reset_index(drop=True)
    y = np.arange(len(df_sorted))
    h = 0.36

    fig, ax = plt.subplots(figsize=(15, 11), dpi=300)
    fig.patch.set_facecolor("white")
    ax.set_facecolor("white")

    ax.barh(y - h/2, df_sorted["pct_25"], height=h, color="#90CAF9", label=f"2025 (GT 6.9, průměr {mean25:.2f}%)", zorder=3)
    ax.barh(y + h/2, df_sorted["pct_26"], height=h, color="#1A73E8", label=f"2026 (GT 7, průměr {mean26:.2f}%)", zorder=3)

    ax.axvline(mean25, color="#1976D2", linestyle=":", linewidth=1.5, alpha=0.8, zorder=2)
    ax.axvline(mean26, color="#EA4335", linestyle="--", linewidth=1.8, alpha=0.8, zorder=2)

    ax.set_yticks(y)
    ax.set_yticklabels(df_sorted["label"], fontsize=10)
    ax.set_xlabel("% Času Stráveného Prací", fontsize=11, fontweight="bold", color="#444444")
    ax.xaxis.set_major_formatter(mtick.PercentFormatter(decimals=1))
    ax.grid(axis="x", color="#EEEEEE", linestyle="-", zorder=1)

    for s in ["top", "right"]: ax.spines[s].set_visible(False)
    for s in ["left", "bottom"]: ax.spines[s].set_color("#CCCCCC")

    for i, r in df_sorted.iterrows():
        delta = r["pct_delta"]
        b_col = "#2E7D32" if delta > 0.05 else ("#C62828" if delta < -0.05 else "#555555")
        b_txt = f"{delta:+.2f}%"
        max_x = max(r["pct_25"], r["pct_26"])
        ax.text(max_x + 0.08, y[i], b_txt, va="center", fontsize=9, fontweight="bold", color=b_col,
                bbox=dict(boxstyle="round,pad=0.25", facecolor="#F8F9FA", edgecolor=b_col, alpha=0.9), zorder=5)

    ax.set_xlim(0, max(df_both["pct_25"].max(), df_both["pct_26"].max()) * 1.22)
    ax.set_title("Garage Trip Chores: Srovnání Ročníků 2025 vs 2026 (% Času Práce)", fontsize=15, fontweight="bold", pad=15, loc="left")
    ax.legend(loc="lower right", frameon=True, facecolor="white", edgecolor="#DADCE0", fontsize=10.5)

    plt.tight_layout()
    plt.savefig(output_file, dpi=300)
    plt.close()
    print(f"Saved: {output_file}")

def plot_multiyear_aggregate(df_both, mean_comb, std_comb, output_file):
    """
    Multi-year combined % bar chart showing multi-year fairness convergence.
    """
    plt.style.use("default")
    df_sorted = df_both.sort_values(by="combined_pct", ascending=False).reset_index(drop=True)

    fig, ax = plt.subplots(figsize=(15, 8.5), dpi=300)
    fig.patch.set_facecolor("white")
    ax.set_facecolor("white")

    x = np.arange(len(df_sorted))
    bars = ax.bar(x, df_sorted["combined_pct"], width=0.58, color="#0D47A1", label="Kombinované % práce (2025 + 2026)", zorder=3)
    ax.axhline(mean_comb, color="#EA4335", linewidth=2.2, label=f"Dvouletý průměr ({mean_comb:.2f}%)", zorder=4)

    ax.set_ylim(0, max(df_sorted["combined_pct"].max() * 1.18, 3.0))
    ax.yaxis.set_major_formatter(mtick.PercentFormatter(decimals=2))
    ax.grid(axis="y", color="#E0E0E0", linestyle="-", linewidth=1, zorder=1)
    ax.set_axisbelow(True)

    for s in ["top", "right"]: ax.spines[s].set_visible(False)
    for s in ["left", "bottom"]: ax.spines[s].set_color("#CCCCCC")

    ax.set_xticks(x)
    ax.set_xticklabels(df_sorted["label"], rotation=50, ha="right", fontsize=9.5)

    for bar in bars:
        h = bar.get_height()
        ax.text(bar.get_x() + bar.get_width()/2.0, h + 0.04, f"{h:.2f}%", ha="center", va="bottom", fontsize=8.5, color="#0D47A1", fontweight="bold",
                bbox=dict(boxstyle="round,pad=0.15", facecolor="white", edgecolor="none", alpha=0.85), zorder=6)

    # SD Box
    in_sd = (df_sorted["combined_pct"] >= mean_comb - std_comb) & (df_sorted["combined_pct"] <= mean_comb + std_comb)
    sd_idx = np.where(in_sd)[0]
    if len(sd_idx) > 0:
        first_i, last_i = sd_idx[0], sd_idx[-1]
        rect_x = first_i - 0.45
        rect_w = (last_i - first_i) + 0.90
        rect_y1 = max(0.0, mean_comb - std_comb)
        rect_y2 = mean_comb + std_comb + 0.12
        rect = patches.Rectangle((rect_x, rect_y1), rect_w, rect_y2 - rect_y1, linewidth=2.5, edgecolor="#EA4335", facecolor="none", zorder=5,
                                 label=f"±1 SD pásmo ({len(sd_idx)}/{len(df_sorted)} lidí, [{mean_comb-std_comb:.2f}% - {mean_comb+std_comb:.2f}%])")
        ax.add_patch(rect)

    ax.legend(loc="upper center", bbox_to_anchor=(0.5, 1.08), ncol=3, frameon=False, fontsize=11)
    plt.tight_layout()
    plt.savefig(output_file, dpi=300)
    plt.close()
    print(f"Saved: {output_file}")

def plot_evolution_scatter(df_both, mean25, mean26, output_file):
    """
    Scatter plot with 4 behavioral quadrants and bubble sizes proportional to total minutes.
    """
    plt.style.use("default")
    fig, ax = plt.subplots(figsize=(12, 10), dpi=300)
    fig.patch.set_facecolor("white")
    ax.set_facecolor("white")

    max_v = max(df_both["pct_25"].max(), df_both["pct_26"].max()) * 1.15
    ax.set_xlim(-0.1, max_v)
    ax.set_ylim(-0.1, max_v)

    # Shaded Quadrants
    ax.fill_between([mean25, max_v], mean26, max_v, color="#E8F5E9", alpha=0.45, zorder=1) # Hardcore
    ax.fill_between([-0.1, mean25], mean26, max_v, color="#E3F2FD", alpha=0.45, zorder=1) # Skokani
    ax.fill_between([-0.1, mean25], -0.1, mean26, color="#F3E5F5", alpha=0.45, zorder=1) # Zen
    ax.fill_between([mean25, max_v], -0.1, mean26, color="#FFF3E0", alpha=0.45, zorder=1) # Dovolenkáři

    # Labels
    ax.text(max_v - 0.1, max_v - 0.1, "HARDCORE DŘÍČI\n(Nadprůměr v obou ročnících)", ha="right", va="top", fontsize=11, fontweight="bold", color="#1B5E20")
    ax.text(0.05, max_v - 0.1, "SKOKANI ROKU\n(Výrazné zlepšení v 2026)", ha="left", va="top", fontsize=11, fontweight="bold", color="#0D47A1")
    ax.text(0.05, 0.05, "ZEN MÓD\n(Klidný stabilní režim)", ha="left", va="bottom", fontsize=11, fontweight="bold", color="#4A148C")
    ax.text(max_v - 0.1, 0.05, "DOVOLENKÁŘI\n(Loni dříči, letos oraz)", ha="right", va="bottom", fontsize=11, fontweight="bold", color="#E65100")

    ax.plot([0, max_v], [0, max_v], color="#777777", linestyle="--", linewidth=1.5, label="Rovnost 2025 = 2026", zorder=2)
    ax.axvline(mean25, color="#1976D2", linestyle=":", linewidth=1.2, label=f"Průměr 2025 ({mean25:.2f}%)", zorder=2)
    ax.axhline(mean26, color="#EA4335", linestyle=":", linewidth=1.2, label=f"Průměr 2026 ({mean26:.2f}%)", zorder=2)

    sizes = (df_both["total_min"] * 2.2) + 120
    sc = ax.scatter(df_both["pct_25"], df_both["pct_26"], s=sizes, c=df_both["pct_delta"], cmap="coolwarm", edgecolors="#222222", linewidth=1.5, zorder=4)

    cbar = plt.colorbar(sc, ax=ax, shrink=0.8, pad=0.03)
    cbar.set_label("Změna % (2026 − 2025)", fontsize=10, fontweight="bold")

    for _, r in df_both.iterrows():
        ax.annotate(r["label"], (r["pct_25"], r["pct_26"]), textcoords="offset points", xytext=(0, 10),
                    ha="center", fontsize=9, fontweight="bold", color="#202124",
                    bbox=dict(boxstyle="round,pad=0.2", facecolor="white", edgecolor="#CCCCCC", alpha=0.85), zorder=5)

    ax.set_xlabel("% Času Práce v Roce 2025 (GT 6.9)", fontsize=12, fontweight="bold", color="#333333")
    ax.set_ylabel("% Času Práce v Roce 2026 (GT 7)", fontsize=12, fontweight="bold", color="#333333")
    ax.xaxis.set_major_formatter(mtick.PercentFormatter(decimals=1))
    ax.yaxis.set_major_formatter(mtick.PercentFormatter(decimals=1))
    ax.set_title("Evoluce Účastníků: Chores 2025 vs 2026", fontsize=15, fontweight="bold", pad=15)
    ax.grid(True, color="#E0E0E0", linestyle="-", zorder=1)
    ax.legend(loc="lower right", frameon=True, facecolor="white", edgecolor="#DADCE0", fontsize=9.5)

    plt.tight_layout()
    plt.savefig(output_file, dpi=300)
    plt.close()
    print(f"Saved: {output_file}")

def plot_macro_trends(stats25, stats26, output_file):
    """
    4-panel visual comparison of macro trends across 2025 and 2026.
    """
    plt.style.use("default")
    fig, axes = plt.subplots(2, 2, figsize=(13, 9), dpi=300)
    fig.patch.set_facecolor("#F8F9FA")

    years = ["2025 (GT 6.9)", "2026 (GT 7)"]
    colors = ["#90CAF9", "#1A73E8"]

    # 1. Total work hours
    ax1 = axes[0, 0]
    ax1.set_facecolor("white")
    hours = [stats25["total_worked_hours"], stats26["total_worked_hours"]]
    b1 = ax1.bar(years, hours, color=colors, width=0.5, zorder=3)
    ax1.set_title("Celkový Odpracovaný Čas (Hodiny)", fontsize=12, fontweight="bold", pad=10)
    ax1.set_ylabel("Hodiny", fontsize=10)
    ax1.grid(axis="y", color="#EEEEEE", zorder=1)
    for bar in b1:
        h = bar.get_height()
        ax1.text(bar.get_x() + bar.get_width()/2, h + 0.8, f"{h:.1f}h", ha="center", va="bottom", fontsize=11, fontweight="bold", color="#202124")
    ax1.set_ylim(0, max(hours) * 1.25)

    # 2. Total completed chores
    ax2 = axes[0, 1]
    ax2.set_facecolor("white")
    chores = [stats25["total_chores"], stats26["total_chores"]]
    b2 = ax2.bar(years, chores, color=colors, width=0.5, zorder=3)
    ax2.set_title("Počet Splněných Úkolů (Chores)", fontsize=12, fontweight="bold", pad=10)
    ax2.set_ylabel("Počet Úkolů", fontsize=10)
    ax2.grid(axis="y", color="#EEEEEE", zorder=1)
    for bar in b2:
        h = bar.get_height()
        ax2.text(bar.get_x() + bar.get_width()/2, h + 2.5, f"{int(h)}", ha="center", va="bottom", fontsize=11, fontweight="bold", color="#202124")
    ax2.set_ylim(0, max(chores) * 1.25)

    # 3. Ack rate
    ax3 = axes[1, 0]
    ax3.set_facecolor("white")
    acks = [stats25["ack_rate_pct"], stats26["ack_rate_pct"]]
    b3 = ax3.bar(years, acks, color=["#34A853", "#1B5E20"], width=0.5, zorder=3)
    ax3.set_title("Míra Potvrzení Úkolů (Ack Rate %)", fontsize=12, fontweight="bold", pad=10)
    ax3.set_ylabel("% Akceptováno", fontsize=10)
    ax3.grid(axis="y", color="#EEEEEE", zorder=1)
    for bar in b3:
        h = bar.get_height()
        ax3.text(bar.get_x() + bar.get_width()/2, h + 0.6, f"{h:.1f}%", ha="center", va="bottom", fontsize=11, fontweight="bold", color="#202124")
    ax3.set_ylim(0, max(acks) * 1.3)

    # 4. Self-reported sessions
    ax4 = axes[1, 1]
    ax4.set_facecolor("white")
    self_rep = [stats25["self_reported_sessions"], stats26["self_reported_sessions"]]
    b4 = ax4.bar(years, self_rep, color=["#FBBC05", "#F29900"], width=0.5, zorder=3)
    ax4.set_title("Dobrovolně Nahlášené Úkoly (Self-Reported)", fontsize=12, fontweight="bold", pad=10)
    ax4.set_ylabel("Počet Sessions", fontsize=10)
    ax4.grid(axis="y", color="#EEEEEE", zorder=1)
    for bar, val in zip(b4, self_rep):
        ax4.text(bar.get_x() + bar.get_width()/2, val + 1.2, f"{int(val)}", ha="center", va="bottom", fontsize=11, fontweight="bold", color="#202124")
    ax4.set_ylim(0, max(self_rep) * 1.3)

    for ax in [ax1, ax2, ax3, ax4]:
        for s in ["top", "right"]: ax.spines[s].set_visible(False)
        for s in ["left", "bottom"]: ax.spines[s].set_color("#CCCCCC")

    plt.suptitle("Garage Trip: Makro Trendy Mezi Ročníky 2025 a 2026", fontsize=16, fontweight="bold", y=0.98)
    plt.tight_layout(rect=[0, 0.03, 1, 0.95])
    plt.savefig(output_file, dpi=300)
    plt.close()
    print(f"Saved: {output_file}")

def generate_interactive_html(df26, df_both, stats26, stats25, stats_comb, output_file):
    """
    Generates a full standalone HTML dashboard with interactive tables and charts.
    """
    u26_json = df26.to_dict(orient="records")
    both_json = df_both.sort_values(by="combined_pct", ascending=False).to_dict(orient="records")

    html = f"""<!DOCTYPE html>
<html lang="cs">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Garage Trip Chores - Velké Srovnání 2025 vs 2026</title>
    <style>
        :root {{
            --primary: #1A73E8;
            --success: #34A853;
            --warning: #FBBC05;
            --danger: #EA4335;
            --bg: #F8F9FA;
            --card-bg: #FFFFFF;
            --text-dark: #202124;
            --text-muted: #5F6368;
            --border: #DADCE0;
        }}
        * {{ box-sizing: border-box; margin: 0; padding: 0; }}
        body {{
            font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, Helvetica, Arial, sans-serif;
            background-color: var(--bg);
            color: var(--text-dark);
            line-height: 1.5;
            padding: 24px;
        }}
        .container {{ max-width: 1300px; margin: 0 auto; }}
        header {{
            background: linear-gradient(135deg, #0D47A1, #1A73E8);
            color: white;
            padding: 32px;
            border-radius: 16px;
            margin-bottom: 24px;
            box-shadow: 0 4px 12px rgba(26,115,232,0.15);
        }}
        header h1 {{ font-size: 28px; font-weight: 700; margin-bottom: 8px; }}
        header p {{ font-size: 15px; opacity: 0.9; }}
        
        .kpi-grid {{
            display: grid;
            grid-template-columns: repeat(auto-fit, minmax(210px, 1fr));
            gap: 16px;
            margin-bottom: 24px;
        }}
        .kpi-card {{
            background: var(--card-bg);
            padding: 20px;
            border-radius: 12px;
            border: 1px solid var(--border);
            box-shadow: 0 1px 3px rgba(0,0,0,0.05);
        }}
        .kpi-title {{ font-size: 13px; color: var(--text-muted); text-transform: uppercase; letter-spacing: 0.5px; font-weight: 600; margin-bottom: 6px; }}
        .kpi-value {{ font-size: 26px; font-weight: 700; color: var(--text-dark); }}
        .kpi-sub {{ font-size: 12px; color: var(--text-muted); margin-top: 4px; }}
        
        .card {{
            background: var(--card-bg);
            border-radius: 12px;
            border: 1px solid var(--border);
            padding: 24px;
            margin-bottom: 24px;
            box-shadow: 0 1px 3px rgba(0,0,0,0.05);
        }}
        .card h2 {{ font-size: 19px; font-weight: 600; margin-bottom: 16px; color: #1F2937; }}
        
        table {{ width: 100%; border-collapse: collapse; font-size: 14px; margin-top: 10px; }}
        th, td {{ padding: 10px 12px; text-align: left; border-bottom: 1px solid var(--border); }}
        th {{ background-color: #F1F3F4; font-weight: 600; color: var(--text-dark); }}
        tr:hover {{ background-color: #F8F9FA; }}
        
        .badge {{
            display: inline-block;
            padding: 2px 8px;
            border-radius: 12px;
            font-size: 12px;
            font-weight: 600;
        }}
        .badge-up {{ background-color: #E6F4EA; color: #137333; }}
        .badge-down {{ background-color: #FCE8E6; color: #C5221F; }}
        .badge-neutral {{ background-color: #F1F3F4; color: #5F6368; }}

        .chart-img {{
            width: 100%;
            height: auto;
            border-radius: 8px;
            border: 1px solid var(--border);
            margin-bottom: 16px;
        }}
        .grid-2 {{ display: grid; grid-template-columns: 1fr 1fr; gap: 20px; }}
        @media (max-width: 900px) {{ .grid-2 {{ grid-template-columns: 1fr; }} }}
    </style>
</head>
<body>
    <div class="container">
        <header>
            <h1>🧹 Garage Trip Chores: Velké Srovnání 2025 vs 2026</h1>
            <p>Souhrnný report o spravedlnosti dělby práce, evoluci flákačů a all-time síni slávy mezi Garage Trip 6.9 a Garage Trip 7.</p>
        </header>

        <div class="kpi-grid">
            <div class="kpi-card">
                <div class="kpi-title">Odpracovaný Čas (2026)</div>
                <div class="kpi-value">{stats26['total_worked_hours']:.1f} h</div>
                <div class="kpi-sub">+17% oproti 2025 ({stats25['total_worked_hours']:.1f} h)</div>
            </div>
            <div class="kpi-card">
                <div class="kpi-title">Průměrné % Práce</div>
                <div class="kpi-value">{stats26['mean_pct_working']:.2f}%</div>
                <div class="kpi-sub">2025: {stats25['mean_pct_working']:.2f}% | Delta: +{stats26['mean_pct_working'] - stats25['mean_pct_working']:.2f}%</div>
            </div>
            <div class="kpi-card">
                <div class="kpi-title">V Pásmu Férovosti (±1 SD)</div>
                <div class="kpi-value">{stats26['in_sd_range_pct']}%</div>
                <div class="kpi-sub">{stats26['in_sd_range_count']}/{stats26['users_count']} lidí (2025: {stats25['in_sd_range_pct']}%)</div>
            </div>
            <div class="kpi-card">
                <div class="kpi-title">Míra Potvrzení Úkolů</div>
                <div class="kpi-value">{stats26['ack_rate_pct']:.1f}%</div>
                <div class="kpi-sub">+80% skok (2025: {stats25['ack_rate_pct']:.1f}%)</div>
            </div>
            <div class="kpi-card">
                <div class="kpi-title">Mr. Average 2026</div>
                <div class="kpi-value">{stats26['most_average_user']}</div>
                <div class="kpi-sub">{stats26['most_average_pct']:.2f}% (2025 král: Kuře 1.04%)</div>
            </div>
        </div>

        <div class="card">
            <h2>📊 Klíčové Grafy Dvouletého Srovnání</h2>
            <div class="grid-2">
                <div>
                    <h3 style="margin-bottom:8px;">Meziroční Srovnání % Práce (18 Navrátilců)</h3>
                    <img src="yoy_comparison.png" alt="YoY Comparison" class="chart-img">
                </div>
                <div>
                    <h3 style="margin-bottom:8px;">Evoluce Účastníků (Kvadranty Dříčů & Skokanů)</h3>
                    <img src="evolution_scatter.png" alt="Evolution Scatter" class="chart-img">
                </div>
            </div>
            <div class="grid-2" style="margin-top:16px;">
                <div>
                    <h3 style="margin-bottom:8px;">All-Time Dvouleté Kombinované Pásmo Férovosti</h3>
                    <img src="multiyear_aggregate.png" alt="Multiyear Aggregate" class="chart-img">
                </div>
                <div>
                    <h3 style="margin-bottom:8px;">Vývoj Makro Metrik (2025 vs 2026)</h3>
                    <img src="macro_trends.png" alt="Macro Trends" class="chart-img">
                </div>
            </div>
        </div>

        <div class="card">
            <h2>🏆 All-Time Síň Slávy: Dvouletý Kombinovaný Žebříček (18 Účastníků)</h2>
            <p style="color:var(--text-muted); margin-bottom:12px;">Seřazeno podle celkového kombinovaného % času věnovaného práci přes oba ročníky (2025 + 2026).</p>
            <table>
                <thead>
                    <tr>
                        <th>#</th>
                        <th>Účastník</th>
                        <th>Kombinované %</th>
                        <th>Celkem Práce</th>
                        <th>Celkem Úkoly</th>
                        <th>2025 %</th>
                        <th>2026 %</th>
                        <th>Delta %</th>
                    </tr>
                </thead>
                <tbody>"""
    
    for i, r in enumerate(both_json):
        delta = r["pct_delta"]
        b_cls = "badge-up" if delta > 0.05 else ("badge-down" if delta < -0.05 else "badge-neutral")
        html += f"""
                    <tr>
                        <td><b>{i+1}</b></td>
                        <td><b>{r['label']}</b></td>
                        <td><b>{r['combined_pct']:.2f}%</b></td>
                        <td>{int(r['total_min'])} min ({(r['total_min']/60.0):.1f}h)</td>
                        <td>{int(r['total_chores'])}</td>
                        <td>{r['pct_25']:.2f}%</td>
                        <td>{r['pct_26']:.2f}%</td>
                        <td><span class="badge {b_cls}">{delta:+.2f}%</span></td>
                    </tr>"""
                    
    html += """
                </tbody>
            </table>
        </div>
    </div>
</body>
</html>"""
    
    with open(output_file, "w", encoding="utf-8") as f:
        f.write(html)
    print(f"Saved: {output_file}")

def generate_pdf_report(df26, df_both, stats26, stats25, stats_comb, output_pdf, charts_dir):
    """
    Generates a professional, attendee-facing multi-page PDF report using WeasyPrint.
    Strictly attendee-oriented: no mention of internal tools or AI models.
    """
    both_sorted = df_both.sort_values(by="combined_pct", ascending=False).reset_index(drop=True)
    
    # Absolute paths for WeasyPrint
    p_pct26 = os.path.abspath(os.path.join(charts_dir, "pct_spent_working.png"))
    p_dist26 = os.path.abspath(os.path.join(charts_dir, "distribution_histogram.png"))
    p_yoy = os.path.abspath(os.path.join(charts_dir, "yoy_comparison.png"))
    p_scatter = os.path.abspath(os.path.join(charts_dir, "evolution_scatter.png"))
    p_agg = os.path.abspath(os.path.join(charts_dir, "multiyear_aggregate.png"))
    p_trends = os.path.abspath(os.path.join(charts_dir, "macro_trends.png"))

    pdf_html = f"""<!DOCTYPE html>
<html lang="cs">
<head>
<meta charset="utf-8">
<style>
@page {{
    size: A4 portrait;
    margin: 1.2cm;
    @bottom-right {{
        content: "Strana " counter(page) " z " counter(pages);
        font-size: 8pt;
        color: #777777;
    }}
    @bottom-left {{
        content: "Garage Trip Chores • Report 2025 & 2026";
        font-size: 8pt;
        color: #777777;
    }}
}}
body {{
    font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, Helvetica, Arial, sans-serif;
    color: #202124;
    line-height: 1.35;
    font-size: 9.5pt;
}}
.page {{
    page-break-after: always;
}}
.page:last-child {{
    page-break-after: avoid;
}}
.header-box {{
    background: linear-gradient(135deg, #0D47A1, #1976D2);
    color: white;
    padding: 20px;
    border-radius: 8px;
    margin-bottom: 16px;
}}
.header-box h1 {{
    font-size: 20pt;
    margin-bottom: 4px;
    font-weight: 700;
}}
.header-box p {{
    font-size: 10pt;
    opacity: 0.92;
}}
h2 {{
    font-size: 13pt;
    font-weight: 700;
    color: #1A73E8;
    border-bottom: 1.5px solid #E0E0E0;
    padding-bottom: 3px;
    margin-top: 14px;
    margin-bottom: 10px;
}}
.kpi-row {{
    display: flex;
    justify-content: space-between;
    margin-bottom: 14px;
}}
.kpi-box {{
    flex: 1;
    background: #F8F9FA;
    border: 1px solid #DADCE0;
    border-radius: 6px;
    padding: 10px;
    margin-right: 8px;
    text-align: center;
}}
.kpi-box:last-child {{
    margin-right: 0;
}}
.kpi-val {{
    font-size: 16pt;
    font-weight: bold;
    color: #1A73E8;
}}
.kpi-lbl {{
    font-size: 7.5pt;
    text-transform: uppercase;
    color: #5F6368;
    font-weight: 600;
}}
.kpi-sub {{
    font-size: 7.5pt;
    color: #34A853;
    font-weight: 500;
}}
.quote-box {{
    background: #E8F0FE;
    border-left: 4px solid #1A73E8;
    padding: 10px 14px;
    border-radius: 0 6px 6px 0;
    font-size: 9pt;
    color: #174EA6;
    margin-bottom: 14px;
}}
table {{
    width: 100%;
    border-collapse: collapse;
    font-size: 7.5pt;
    margin-top: 4px;
}}
th, td {{
    padding: 3.5px 5px;
    border-bottom: 1px solid #E0E0E0;
    text-align: left;
}}
th {{
    background: #F1F3F4;
    font-weight: 600;
    color: #202124;
}}
tr:nth-child(even) {{
    background: #FAFAFA;
}}
.badge-up {{ color: #137333; font-weight: bold; }}
.badge-down {{ color: #C5221F; font-weight: bold; }}
.badge-neut {{ color: #5F6368; }}
.award-card {{
    background: #FFFFFF;
    border: 1px solid #DADCE0;
    border-radius: 6px;
    padding: 10px 12px;
    margin-bottom: 10px;
    border-left: 4px solid #1A73E8;
}}
.award-title {{
    font-size: 10.5pt;
    font-weight: bold;
    color: #202124;
    margin-bottom: 2px;
}}
.award-desc {{
    font-size: 8.5pt;
    color: #444444;
}}
.img-center {{
    text-align: center;
    margin: 8px 0;
}}
.img-center img {{
    max-width: 98%;
    max-height: 380px;
    border-radius: 4px;
    border: 1px solid #E0E0E0;
}}
</style>
</head>
<body>

<!-- STRANA 1: Titul, Manažerské shrnutí, Srovnání 2025 vs 2026 -->
<div class="page">
    <div class="header-box">
        <h1>Garage Trip Chores: Velké Srovnání 2025 vs 2026</h1>
        <p>Dvouletá bilance spravedlnosti dělby práce, evoluce flákačů a all-time síň slávy (GT 6.9 & GT 7)</p>
    </div>

    <div class="kpi-row">
        <div class="kpi-box">
            <div class="kpi-val">{stats26['total_worked_hours']:.1f} h</div>
            <div class="kpi-lbl">Celkem práce (2026)</div>
            <div class="kpi-sub">+17% vs 2025 ({stats25['total_worked_hours']:.1f}h)</div>
        </div>
        <div class="kpi-box">
            <div class="kpi-val">{stats26['mean_pct_working']:.2f}%</div>
            <div class="kpi-lbl">Průměrné % práce</div>
            <div class="kpi-sub">+0.23% (2025: {stats25['mean_pct_working']:.2f}%)</div>
        </div>
        <div class="kpi-box">
            <div class="kpi-val">{stats26['in_sd_range_pct']}%</div>
            <div class="kpi-lbl">V pásmu spravedlnosti</div>
            <div class="kpi-sub">{stats26['in_sd_range_count']}/{stats26['users_count']} lidí (2025: {stats25['in_sd_range_pct']}%)</div>
        </div>
        <div class="kpi-box">
            <div class="kpi-val">{stats26['ack_rate_pct']:.1f}%</div>
            <div class="kpi-lbl">Ochota potvrdit bota</div>
            <div class="kpi-sub">+80% skok (2025: {stats25['ack_rate_pct']:.1f}%)</div>
        </div>
    </div>

    <div class="quote-box">
        <b>Hlavní zjištění:</b> Chores bot letos dosáhl svého dosud nejspravedlivějšího výsledku. V pásmu směrodatné odchylky skončilo <b>77.3% účastníků</b> (oproti loňským 62.5%). Zároveň se výrazně zvýšila disciplína – lidé potvrdili o 80% více přidělených úkolů a počet dobrovolně nahlášených prací stoupl z 12 na 51!
    </div>

    <h2>Makro Trendy Mezi Ročníky</h2>
    <div class="img-center">
        <img src="file://{p_trends}" style="max-height: 380px;">
    </div>
</div>

<!-- STRANA 2: Ročník 2026 – Grafy a Normální rozdělení -->
<div class="page">
    <h2>Garage Trip 2026: % Času Stráveného Prací a Zóna Spravedlnosti</h2>
    <p style="font-size:8pt; color:#555555; margin-bottom:2px;">Červený obdélník vyznačuje pásmo ±1 směrodatné odchylky od průměru ({stats26['mean_pct_working']:.2f}% ± {stats26['std_pct_working_sample']:.2f}%). Zahrnuje 17 z 22 účastníků.</p>
    <div class="img-center">
        <img src="file://{p_pct26}" style="max-height: 275px;">
    </div>

    <h2>Rozdělení Hustoty Četnosti a Normální Křivka (2026)</h2>
    <div class="img-center">
        <img src="file://{p_dist26}" style="max-height: 275px;">
    </div>
</div>

<!-- STRANA 3: Dvouleté Srovnání – Meziroční graf a Kvadranty -->
<div class="page">
    <h2>Meziroční Srovnání % Práce: 2025 vs 2026 (18 Účastníků)</h2>
    <p style="font-size:8pt; color:#555555; margin-bottom:2px;">Porovnání výkonu 18 lidí, kteří se zúčastnili obou ročníků, včetně vyčísleného rozdílu v procentních bodech.</p>
    <div class="img-center">
        <img src="file://{p_yoy}" style="max-height: 275px;">
    </div>

    <h2>Evoluce Účastníků: Behaviorální Kvadranty</h2>
    <div class="img-center">
        <img src="file://{p_scatter}" style="max-height: 275px;">
    </div>
</div>

<!-- STRANA 4: All-Time Dvouletá Síň Slávy -->
<div class="page">
    <h2>All-Time Kombinované Pásmo Férovosti (2025 + 2026)</h2>
    <p style="font-size:8pt; color:#555555; margin-bottom:2px;">V součtu dvou let se díky zákonu velkých čísel odchylka snížila na pouhých {stats_comb['std_pct']:.2f}%. Férovost se v dlouhém horizontu vyrovnává.</p>
    <div class="img-center">
        <img src="file://{p_agg}" style="max-height: 175px;">
    </div>

    <h2>Dvouletý Žebříček Účastníků (All-Time Leaderboard)</h2>
    <table>
        <thead>
            <tr>
                <th>#</th>
                <th>Účastník</th>
                <th>Kombinované %</th>
                <th>Celkem Minut</th>
                <th>Celkem Úkolů</th>
                <th>Pobyt</th>
                <th>2025 %</th>
                <th>2026 %</th>
                <th>Posun (Delta)</th>
            </tr>
        </thead>
        <tbody>"""

    for i, r in both_sorted.iterrows():
        d = r["pct_delta"]
        d_cls = "badge-up" if d > 0.05 else ("badge-down" if d < -0.05 else "badge-neut")
        pdf_html += f"""
            <tr>
                <td><b>{i+1}</b></td>
                <td><b>{r['label']}</b></td>
                <td><b>{r['combined_pct']:.2f}%</b></td>
                <td>{int(r['total_min'])} min ({(r['total_min']/60.0):.1f}h)</td>
                <td>{int(r['total_chores'])}</td>
                <td>{(r['total_pres']/60.0):.1f}h</td>
                <td>{r['pct_25']:.2f}%</td>
                <td>{r['pct_26']:.2f}%</td>
                <td><span class="{d_cls}">{d:+.2f}%</span></td>
            </tr>"""

    pdf_html += f"""
        </tbody>
    </table>
</div>

<!-- STRANA 5: Síně slávy, Zábavné ceny & Wall of Shame -->
<div class="page">
    <h2>🏆 Oficiální Dvouleté Ceny & Síně Slávy</h2>

    <div class="award-card" style="border-left-color: #34A853;">
        <div class="award-title">👑 All-Time Chore King (Grand MVP): @tivvit (vit listik)</div>
        <div class="award-desc">Neuvěřitelná konzistence na špici: 222 min v roce 2025 (2.51%) a 216 min v roce 2026 (2.34%). Celkem <b>438 minut (7.3 hodiny) čisté práce</b> v 24 úkolech. Absolutní pilíř chaty.</div>
    </div>

    <div class="award-card" style="border-left-color: #1A73E8;">
        <div class="award-title">🚀 Skokan Roku (Comeback of the Century): @Fisa (Zbyněk Fišer)</div>
        <div class="award-desc">Z loňského solidního průměru 1.13% (100 min) vystřelil letos na <b>2.77% (256 min práce)</b>! Skok o <b>+1.64 procentního bodu</b> a absolutní vítěz ročníku 2026. Těsně v patách mu byl @Rasťo (+1.41% posun z 25 na 172 min!).</div>
    </div>

    <div class="award-card" style="border-left-color: #FBBC05;">
        <div class="award-title">👑 Předání Koruny Mr. Average: @Kuře ➡️ @Liennie.sh</div>
        <div class="award-desc">Loňský pan Průměrný @Kuře (Martin Kolek) s 1.04% letos zabral na 1.81% a korunu přenechal. Novým králem průměrnosti roku 2026 je <b>@Liennie.sh (Martin T.)</b> s 1.33% (přesně u průměru 1.27%). Z loňských 25 min si polepšil o +1.05%!</div>
    </div>

    <div class="award-card" style="border-left-color: #E65100;">
        <div class="award-title">🏖️ Zasloužený Odpočinek (Dovolenkář Roku): @David V. (Destil)</div>
        <div class="award-desc">V roce 2025 dříč na bedně s 1.87% (165 min), letos nohy na stůl a pohodových 0.64% (50 min). Delta <b>-1.23%</b>. Loni si to odmakal dopředu!</div>
    </div>

    <div class="award-card" style="border-left-color: #5F6368;">
        <div class="award-title">🥷 Stealth Master: @Darik</div>
        <div class="award-desc">Z loňských poctivých 100 min (1.13%) přešel na absolutní stealth režim: 2 minuty práce (0.05%), 9 timeoutů, 0 potvrzení a v úterý v 9:00 ráno nenápadný odjezd. Legenda žije!</div>
    </div>

    <div class="award-card" style="border-left-color: #EA4335;">
        <div class="award-title">🚨 408 Request Timeout & Wall of Shame</div>
        <div class="award-desc">Absolutní mistryní v ignorování chores bota se stala <b>@Klára Vonšovská</b> s 53 timeouty z 57 přiřazení (93% timeout rate). Na záda jí dýchala @Zuzka se 45 timeouty. Hrdým odmítačem je <b>@jask</b> se 2 statečnými kliknutími na "Refuse".</div>
    </div>
</div>

</body>
</html>"""

    html_tmp = os.path.join(charts_dir, "report_print.html")
    with open(html_tmp, "w", encoding="utf-8") as f:
        f.write(pdf_html)

    # Run WeasyPrint
    weasyprint_bin = "/opt/homebrew/bin/weasyprint"
    if not os.path.exists(weasyprint_bin):
        weasyprint_bin = "weasyprint"
        
    try:
        res = subprocess.run([weasyprint_bin, html_tmp, output_pdf], capture_output=True, text=True, check=True)
        print(f"Saved PDF report: {output_pdf}")
    except Exception as e:
        print(f"Warning: Could not compile PDF via WeasyPrint: {e}")

def main():
    args = parse_args()
    os.makedirs(args.output_dir, exist_ok=True)
    user_map = load_user_map(args.user_map)

    # 1. Load 2026 DB
    if not os.path.exists(args.db_2026):
        sys.exit(f"Error: 2026 database not found at {args.db_2026}")
    con26 = sqlite3.connect(args.db_2026)
    
    # 2026 Cleanup: exclude Anetka per explicit user instructions, end at 2026-09-19 00:00:00
    df26, df_daily26, df_top26 = fetch_year_data(
        con26,
        sample_period_min=args.sample_period_2026,
        user_map=user_map,
        exclude_uids={"1393320844414947481"},
        max_presence_ts="2026-09-19 00:00:00"
    )

    # 2026 Stats
    mean26 = float(df26["pct_working"].mean())
    std_sample26 = float(df26["pct_working"].std(ddof=1))
    std_pop26 = float(df26["pct_working"].std(ddof=0))
    median26 = float(df26["pct_working"].median())
    in_range26 = int(((df26["pct_working"] >= (mean26 - std_sample26)) & (df26["pct_working"] <= (mean26 + std_sample26))).sum())
    closest_idx26 = (df26["pct_working"] - mean26).abs().idxmin()
    mr_avg26 = df26.loc[closest_idx26, "label"]

    total_assign26 = int(df26["total_assignments"].sum())
    total_acked26 = int(df26["acked_count"].sum())
    total_self26 = int(df26["self_reported_count"].sum())

    stats26 = {
        "year": 2026,
        "event": "Garage Trip 7",
        "users_count": len(df26),
        "mean_pct_working": round(mean26, 2),
        "std_pct_working_sample": round(std_sample26, 2),
        "std_pct_working_pop": round(std_pop26, 2),
        "median_pct_working": round(median26, 2),
        "in_sd_range_count": in_range26,
        "in_sd_range_pct": round(in_range26 / len(df26) * 100.0, 1),
        "most_average_user": mr_avg26,
        "most_average_pct": round(float(df26.loc[closest_idx26, "pct_working"]), 2),
        "mean_worked_minutes": round(float(df26["worked_min"].mean()), 1),
        "median_worked_minutes": round(float(df26["worked_min"].median()), 1),
        "total_worked_minutes": round(float(df26["worked_min"].sum()), 1),
        "total_worked_hours": round(float(df26["worked_min"].sum() / 60.0), 1),
        "total_chores": int(len(df_top26)),
        "total_chore_sessions": int(df26["chores_done"].sum()),
        "ack_rate_pct": round(total_acked26 / total_assign26 * 100.0, 1) if total_assign26 > 0 else 0,
        "self_reported_sessions": total_self26
    }

    # 2. Load 2025 DB if available
    has_2025 = os.path.exists(args.db_2025)
    stats25 = {}
    df_both = pd.DataFrame()
    stats_comb = {}

    if has_2025:
        con25 = sqlite3.connect(args.db_2025)
        # Exclude deleted user 1165949512867651615 (16h visitor, 0 tasks)
        df25, df_daily25, df_top25 = fetch_year_data(
            con25,
            sample_period_min=args.sample_period_2025,
            user_map=user_map,
            exclude_uids={"1165949512867651615"},
            max_presence_ts="2025-09-27 00:00:00"
        )

        mean25 = float(df25["pct_working"].mean())
        std_sample25 = float(df25["pct_working"].std(ddof=1))
        in_range25 = int(((df25["pct_working"] >= (mean25 - std_sample25)) & (df25["pct_working"] <= (mean25 + std_sample25))).sum())
        total_assign25 = int(df25["total_assignments"].sum())
        total_acked25 = int(df25["acked_count"].sum())
        total_self25 = int(df25["self_reported_count"].sum())

        stats25 = {
            "year": 2025,
            "event": "Garage Trip 6.9",
            "users_count": len(df25),
            "mean_pct_working": round(mean25, 2),
            "std_pct_working_sample": round(std_sample25, 2),
            "in_sd_range_count": in_range25,
            "in_sd_range_pct": round(in_range25 / len(df25) * 100.0, 1),
            "total_worked_hours": round(float(df25["worked_min"].sum() / 60.0), 1),
            "total_chores": int(len(df_top25)),
            "ack_rate_pct": round(total_acked25 / total_assign25 * 100.0, 1) if total_assign25 > 0 else 0,
            "self_reported_sessions": total_self25
        }

        # Multi-year merge
        df_both = pd.merge(
            df25[["user_id", "pct_working", "worked_min", "presence_min", "chores_done"]],
            df26[["user_id", "pct_working", "worked_min", "presence_min", "chores_done", "label", "short_name"]],
            on="user_id",
            how="inner",
            suffixes=("_25", "_26")
        )
        df_both["pct_delta"] = df_both["pct_working_26"] - df_both["pct_working_25"]
        df_both["pct_25"] = df_both["pct_working_25"]
        df_both["pct_26"] = df_both["pct_working_26"]
        df_both["total_min"] = df_both["worked_min_25"] + df_both["worked_min_26"]
        df_both["total_pres"] = df_both["presence_min_25"] + df_both["presence_min_26"]
        df_both["combined_pct"] = (df_both["total_min"] / df_both["total_pres"]) * 100.0
        df_both["total_chores"] = df_both["chores_done_25"] + df_both["chores_done_26"]

        mean_comb = float(df_both["combined_pct"].mean())
        std_comb = float(df_both["combined_pct"].std(ddof=1))
        in_range_comb = int(((df_both["combined_pct"] >= (mean_comb - std_comb)) & (df_both["combined_pct"] <= (mean_comb + std_comb))).sum())

        stats_comb = {
            "returning_users_count": len(df_both),
            "mean_pct": round(mean_comb, 2),
            "std_pct": round(std_comb, 2),
            "in_sd_count": in_range_comb,
            "in_sd_pct": round(in_range_comb / len(df_both) * 100.0, 1)
        }

    # 3. Output paths
    p_pct26 = os.path.join(args.output_dir, "pct_spent_working.png")
    p_dist26 = os.path.join(args.output_dir, "distribution_histogram.png")
    p_dash26 = os.path.join(args.output_dir, "chores_dashboard.png")
    p_yoy = os.path.join(args.output_dir, "yoy_comparison.png")
    p_agg = os.path.join(args.output_dir, "multiyear_aggregate.png")
    p_scatter = os.path.join(args.output_dir, "evolution_scatter.png")
    p_trends = os.path.join(args.output_dir, "macro_trends.png")
    p_html = os.path.join(args.output_dir, "report.html")
    p_pdf = os.path.join(args.output_dir, "garage_trip_chores_report_2025_2026.pdf")
    p_summary = os.path.join(args.output_dir, "stats_summary.json")
    p_comp_summary = os.path.join(args.output_dir, "stats_comparison.json")

    # 4. Generate Single-Year Charts
    plot_pct_spent_working(df26, mean26, std_sample26, p_pct26)
    plot_distribution(df26, mean26, std_sample26, p_dist26)
    plot_comprehensive_dashboard(df26, df_daily26, df_top26, mean26, std_sample26, p_dash26)

    # 5. Generate Multi-Year Charts if 2025 DB available
    if has_2025 and len(df_both) > 0:
        plot_yoy_comparison(df_both, stats25["mean_pct_working"], stats26["mean_pct_working"], p_yoy)
        plot_multiyear_aggregate(df_both, stats_comb["mean_pct"], stats_comb["std_pct"], p_agg)
        plot_evolution_scatter(df_both, stats25["mean_pct_working"], stats26["mean_pct_working"], p_scatter)
        plot_macro_trends(stats25, stats26, p_trends)

        # Save comparison JSON
        comp_data = {
            "comparison_title": "Garage Trip 6.9 (2025) vs Garage Trip 7 (2026)",
            "stats_2025": stats25,
            "stats_2026": stats26,
            "multiyear_combined": stats_comb,
            "returning_attendees": df_both.sort_values(by="combined_pct", ascending=False)[[
                "user_id", "label", "combined_pct", "total_min", "total_chores", "pct_25", "pct_26", "pct_delta"
            ]].to_dict(orient="records")
        }
        with open(p_comp_summary, "w", encoding="utf-8") as f:
            json.dump(comp_data, f, indent=2, ensure_ascii=False)
        print(f"Saved: {p_comp_summary}")

    # Save summary JSON
    with open(p_summary, "w", encoding="utf-8") as f:
        json.dump(stats26, f, indent=2, ensure_ascii=False)
    print(f"Saved: {p_summary}")

    # Generate HTML & PDF
    generate_interactive_html(df26, df_both, stats26, stats25, stats_comb, p_html)
    if not args.no_pdf and has_2025:
        generate_pdf_report(df26, df_both, stats26, stats25, stats_comb, p_pdf, args.output_dir)

    # Copy to artifacts directory
    artifact_dir = "/Users/tivvit/.gemini/antigravity/brain/8ba1b3f4-f875-4e49-81d1-57aed1679228"
    if os.path.exists(artifact_dir):
        files_to_copy = [p_pct26, p_dist26, p_dash26, p_html, p_summary]
        if has_2025:
            files_to_copy.extend([p_yoy, p_agg, p_scatter, p_trends, p_comp_summary])
            if os.path.exists(p_pdf):
                files_to_copy.append(p_pdf)
        for src in files_to_copy:
            if os.path.exists(src):
                dst = os.path.join(artifact_dir, os.path.basename(src))
                shutil.copy2(src, dst)
                print(f"Copied to artifact dir: {dst}")

    print("\n--- 2026 Statistics ---")
    print(json.dumps(stats26, indent=2, ensure_ascii=False))
    if has_2025:
        print("\n--- Multi-Year Summary ---")
        print(json.dumps(stats_comb, indent=2, ensure_ascii=False))

if __name__ == "__main__":
    main()
