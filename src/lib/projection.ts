import * as d3 from "d3";
import _ from "lodash";
import tippy from "tippy.js";
import COLORS from "./colors";
import { formatCurrency, formatCurrencyCrude, isMobile, tooltip, type Legend } from "./utils";

export interface ProjectionPoint {
  date: Date;
  networth: number;
  investment: number;
}

function computeProjectionPoints(
  currentNetworth: number,
  currentInvestment: number,
  monthlySavings: number,
  horizonMonths: number,
  annualReturnRate: number
): ProjectionPoint[] {
  const monthlyRate = annualReturnRate / 12;
  const points: ProjectionPoint[] = [];
  const origin = new Date();

  let w = currentNetworth;
  let inv = currentInvestment;

  for (let i = 0; i <= horizonMonths; i++) {
    const date = new Date(origin);
    date.setMonth(date.getMonth() + i);
    points.push({ date, networth: w, investment: inv });
    w = w * (1 + monthlyRate) + monthlySavings;
    inv = inv + monthlySavings;
  }
  return points;
}

function drawLine(
  g: d3.Selection<SVGGElement, unknown, null, undefined>,
  points: ProjectionPoint[],
  x: d3.ScaleTime<number, number>,
  y: d3.ScaleLinear<number, number>,
  accessor: (d: ProjectionPoint) => number,
  color: string,
  dashed: boolean
) {
  g.append("path")
    .datum(points)
    .style("stroke", color)
    .style("stroke-width", "1.5")
    .style("fill", "none")
    .style("opacity", dashed ? "0.7" : "1")
    .style("stroke-dasharray", dashed ? "6,4" : null)
    .attr(
      "d",
      d3
        .line<ProjectionPoint>()
        .curve(d3.curveMonotoneX)
        .x((d) => x(d.date))
        .y((d) => y(accessor(d)))
    );
}

export function renderProjection(
  currentNetworth: number,
  currentInvestment: number,
  monthlySavings: number,
  horizonYears: number,
  annualReturnPct: number,
  element: Element,
  historicalPoints: ProjectionPoint[] = []
): { destroy: () => void; legends: Legend[] } {
  const horizonMonths = horizonYears * 12;
  const annualRate = annualReturnPct / 100;
  const projectionPoints = computeProjectionPoints(
    currentNetworth,
    currentInvestment,
    monthlySavings,
    horizonMonths,
    annualRate
  );

  const svg = d3.select(element);
  svg.selectAll("*").remove();

  const right = isMobile() ? 10 : 80;
  const margin = { top: 15, right, bottom: 20, left: 40 };
  const width = Math.max(element.parentElement.clientWidth, 800) - margin.left - margin.right;
  const height = +svg.attr("height") - margin.top - margin.bottom;
  const g = svg.append("g").attr("transform", `translate(${margin.left},${margin.top})`);

  svg.attr("width", width + margin.left + margin.right);

  const xStart = historicalPoints.length > 0 ? historicalPoints[0].date : projectionPoints[0].date;
  const xEnd = projectionPoints[projectionPoints.length - 1].date;

  const allValues = [
    ..._.flatMap(historicalPoints, (p) => [p.networth, p.investment]),
    ..._.flatMap(projectionPoints, (p) => [p.networth, p.investment]),
    0
  ];

  const x = d3.scaleTime().range([0, width]).domain([xStart, xEnd]);
  const y = d3.scaleLinear().range([height, 0]).domain(d3.extent(allValues));

  // Axes
  g.append("g")
    .attr("class", "axis x")
    .attr("transform", `translate(0,${height})`)
    .call(d3.axisBottom(x));

  g.append("g")
    .attr("class", "axis y")
    .call(d3.axisLeft(y).tickSize(-width).tickFormat(formatCurrencyCrude));

  if (!isMobile()) {
    g.append("g")
      .attr("class", "axis y")
      .attr("transform", `translate(${width},0)`)
      .call(d3.axisRight(y).tickPadding(5).tickFormat(formatCurrencyCrude));
  }

  // Gain shading — projection only
  const gainArea = d3
    .area<ProjectionPoint>()
    .curve(d3.curveMonotoneX)
    .x((d) => x(d.date))
    .y0((d) => y(d.investment))
    .y1((d) => y(d.networth));

  g.append("path")
    .datum(projectionPoints)
    .style("fill", COLORS.gain)
    .style("opacity", "0.15")
    .attr("d", gainArea);

  // Historical lines (solid)
  if (historicalPoints.length > 0) {
    drawLine(g, historicalPoints, x, y, (d) => d.investment, COLORS.secondary, false);
    drawLine(g, historicalPoints, x, y, (d) => d.networth, COLORS.primary, false);
  }

  // Projection lines (dashed) — prepend last historical point so lines connect
  const connectInv =
    historicalPoints.length > 0
      ? [historicalPoints[historicalPoints.length - 1], ...projectionPoints]
      : projectionPoints;
  const connectNW = connectInv;

  drawLine(g, connectInv, x, y, (d) => d.investment, COLORS.secondary, true);
  drawLine(g, connectNW, x, y, (d) => d.networth, COLORS.primary, true);

  // "Today" vertical marker
  const todayX = x(projectionPoints[0].date);
  g.append("line")
    .attr("x1", todayX)
    .attr("x2", todayX)
    .attr("y1", 0)
    .attr("y2", height)
    .style("stroke", "#888")
    .style("stroke-width", "1")
    .style("stroke-dasharray", "3,3");

  g.append("text")
    .attr("x", todayX + 4)
    .attr("y", 12)
    .style("fill", "#888")
    .style("font-size", "11px")
    .text("Today");

  // Hover — covers historical + projection points
  const hoverCircle = g.append("circle").attr("r", "3").attr("fill", "none");
  const t = tippy(hoverCircle.node(), { theme: "light", delay: 0, allowHTML: true });

  const allPoints: ["actual" | "projected", ProjectionPoint][] = [
    ...historicalPoints.map((p) => ["actual", p] as ["actual" | "projected", ProjectionPoint]),
    ...projectionPoints.map((p) => ["projected", p] as ["actual" | "projected", ProjectionPoint])
  ];

  const voronoiNW: [number, number][] = allPoints.map(([, d]) => [x(d.date), y(d.networth)]);
  const voronoiInv: [number, number][] = allPoints.map(([, d]) => [x(d.date), y(d.investment)]);
  const voronoi = d3.Delaunay.from(voronoiNW.concat(voronoiInv)).voronoi([0, 0, width, height]);

  const labelFmt = d3.timeFormat("%b %Y");

  type HoverEntry = ["networth" | "investment", "actual" | "projected", ProjectionPoint];
  const hoverData: HoverEntry[] = [
    ...allPoints.map(([kind, p]) => ["networth", kind, p] as HoverEntry),
    ...allPoints.map(([kind, p]) => ["investment", kind, p] as HoverEntry)
  ];

  g.append("g")
    .selectAll("path")
    .data(hoverData)
    .enter()
    .append("path")
    .style("pointer-events", "all")
    .style("fill", "none")
    .attr("d", (_, i) => voronoi.renderCell(i))
    .on("mouseover", (_, [line, kind, d]) => {
      const cy = line === "networth" ? y(d.networth) : y(d.investment);
      const color = line === "networth" ? COLORS.primary : COLORS.secondary;
      hoverCircle.attr("cx", x(d.date)).attr("cy", cy).attr("fill", color);
      const nwLabel = kind === "actual" ? "Net Worth" : "Projected Net Worth";
      const invLabel = kind === "actual" ? "Net Investment" : "Projected Net Investment";
      t.setProps({
        placement: line === "networth" ? "top" : "bottom",
        content: tooltip([
          ["Month", labelFmt(d.date)],
          [nwLabel, [formatCurrency(d.networth), "has-text-weight-bold has-text-right"]],
          [invLabel, [formatCurrency(d.investment), "has-text-weight-bold has-text-right"]]
        ])
      });
      t.show();
    })
    .on("mouseout", () => {
      t.hide();
      hoverCircle.attr("fill", "none");
    });

  const legends: Legend[] = [
    { label: "Net Worth", color: COLORS.primary, shape: "line" },
    { label: "Net Investment", color: COLORS.secondary, shape: "line" },
    { label: "Projected Gain", color: COLORS.gain, shape: "square" }
  ];

  return { destroy: () => t.destroy(), legends };
}
