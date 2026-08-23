#!/usr/bin/env python3
"""Generate the Acme Q2 FY2026 results PDF used as a repo demo.

Produces a report containing a real data table and a vector pie chart, so the
agentic layer can be shown extracting both (table -> markdown table,
figure -> caption entry). Output: acme-q2-results.pdf next to this script.
"""

import os

import matplotlib
from matplotlib.backends.backend_pdf import PdfPages
matplotlib.use("Agg")
import matplotlib.pyplot as plt

HERE = os.path.dirname(os.path.abspath(__file__))
OUT = os.path.join(HERE, "acme-q2-results.pdf")

plt.rcParams.update({"font.family": "DejaVu Sans", "font.size": 11})

segments = ["Platform", "Professional Services", "Hardware"]
revenue = [11.2, 5.1, 2.1]
yoy = ["+15%", "flat", "-9%"]
share = ["61%", "28%", "11%"]

with PdfPages(OUT) as pdf:

    # ---- Page 1: summary + financial table --------------------------------
    fig = plt.figure(figsize=(8.27, 11.69))  # A4 portrait
    fig.text(0.5, 0.94, "ACME Industries plc", ha="center", fontsize=22, weight="bold")
    fig.text(0.5, 0.905, "Q2 FY2026 Results", ha="center", fontsize=17)
    fig.text(0.5, 0.875, "Prepared by the Office of the CFO · August 2026", ha="center",
             fontsize=10, color="#555555")

    intro = (
        "Revenue reached $18.4M in Q2 FY2026, up 12.3% year over year. Gross margin\n"
        "expanded to 61.7% while operating expenses grew more slowly than revenue for\n"
        "the fourth consecutive quarter. Free cash flow was $3.1M and net revenue\n"
        "retention ended the quarter at 117%."
    )
    fig.text(0.08, 0.80, intro, fontsize=11, va="top", linespacing=1.6)

    fig.text(0.08, 0.68, "Revenue by segment ($M)", fontsize=13, weight="bold")
    ax_tbl = fig.add_axes([0.08, 0.50, 0.84, 0.15])
    ax_tbl.axis("off")
    rows = [
        [s, f"{r:.1f}", y, p]
        for s, r, y, p in zip(segments, revenue, yoy, share)
    ]
    tbl = ax_tbl.table(
        cellText=rows,
        colLabels=["Segment", "Revenue ($M)", "YoY", "Share of revenue"],
        loc="center",
        cellLoc="left",
        colWidths=[0.38, 0.20, 0.14, 0.28],
    )
    tbl.auto_set_font_size(False)
    tbl.set_fontsize(11)
    for (r, c), cell in tbl.get_celld().items():
        if r == 0:
            cell.set_facecolor("#dce4ff")
            cell.set_text_props(weight="bold")
        cell.set_edgecolor("#bbbbbb")
        if c > 0:
            cell.PAD = 0.04

    fig.text(0.08, 0.44, "Key metrics", fontsize=13, weight="bold")
    metrics = (
        "Gross margin: 61.7% (+240 bps YoY)\n"
        "Operating margin: 14.2%\n"
        "Customers > $100K ARR: 87 (+14 QoQ)\n"
        "Employee count: 412 (+3% QoQ)"
    )
    fig.text(0.08, 0.42, metrics, fontsize=11, va="top", linespacing=1.7)

    pdf.savefig(fig)
    plt.close(fig)

    # ---- Page 2: pie chart -------------------------------------------------
    fig = plt.figure(figsize=(8.27, 11.69))
    fig.text(0.5, 0.92, "Revenue mix — Q2 FY2026", ha="center", fontsize=16, weight="bold")

    ax = fig.add_axes([0.15, 0.30, 0.7, 0.55])
    colors = ["#4c6ef5", "#74b3ff", "#adb5bd"]
    wedges, texts, autotexts = ax.pie(
        revenue,
        labels=segments,
        colors=colors,
        autopct="%1.0f%%",
        startangle=90,
        counterclock=False,
        textprops={"fontsize": 12},
        wedgeprops={"edgecolor": "white", "linewidth": 2},
    )
    for t in autotexts:
        t.set_color("white")
        t.set_fontweight("bold")
    ax.set_aspect("equal")

    fig.text(
        0.5, 0.24,
        "Figure 1: Revenue mix by segment, Q2 FY2026. Platform remains the largest\n"
        "contributor at 61% of total revenue; Hardware continues its planned decline.",
        ha="center", fontsize=10.5, color="#444444", linespacing=1.5,
    )
    pdf.savefig(fig)
    plt.close(fig)

print(f"wrote {OUT}")
