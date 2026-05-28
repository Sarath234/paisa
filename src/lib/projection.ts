import * as d3 from "d3";
import _ from "lodash";
import tippy from "tippy.js";
import COLORS from "./colors";
import {
  formatCurrency,
  formatCurrencyCrude,
  isMobile,
  svgUrl,
  tooltip,
  type Legend
} from "./utils";

interface ProjectionPoint {
  date: Date;
  networth: number;
  investment: number;
}

function computePoints(
  currentNetworth: number,
  currentInvestment: number,
  monthlySavings: number,
  horizonMonths: number,
  annualReturnRate: number
): ProjectionPoint[] {
  const monthlyRate = annualReturnRate / 12;
  const points: ProjectionPoint[] = [];
  const origin = new Date();
  origin.setDate(1);

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

export function renderProjection(
  currentNetworth: number,
  currentInvestment: number,
  monthlySavings: number,
  horizonYears: number,
  annualReturnPct: number,
  element: Element
): { destroy: () => void; legends: Legend[] } {
  const horizonMonths = horizonYears * 12;
  const annualRate = annualReturnPct / 100;
  const points = computePoints(
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
  const width =
    Math.max(element.parentElement.clientWidth, 800) - margin.left - margin.right;
  const height = +svg.attr("height") - margin.top - margin.bottom;
  const g = svg
    .append("g")
    .attr("transform", `translate(${margin.left},${margin.top})`);

  svg.attr("width", width + margin.left + margin.right);

  const allValues = _.flatMap(points, (p) => [p.networth, p.investment]);
  allValues.push(0);

  const x = d3
    .scaleTime()
    .range([0, width])
    .domain([points[0].date, points[points.length - 1].date]);

  const y = d3.scaleLinear().range([height, 0]).domain(d3.extent(allValues));

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

  // Shaded gain area between investment and networth lines
  const gainAreaID = _.uniqueId("proj-gain");
  const gainArea = d3
    .area<ProjectionPoint>()
    .curve(d3.curveMonotoneX)
    .x((d) => x(d.date))
    .y0((d) => y(d.investment))
    .y1((d) => y(d.networth));

  g.append("path")
    .datum(points)
    .attr("id", gainAreaID)
    .style("fill", COLORS.gain)
    .style("opacity", "0.2")
    .attr("d", gainArea);

  // Net Investment line (dashed)
  g.append("path")
    .datum(points)
    .style("stroke", COLORS.secondary)
    .style("stroke-width", "1.5")
    .style("stroke-dasharray", "6,4")
    .style("fill", "none")
    .attr(
      "d",
      d3
        .line<ProjectionPoint>()
        .curve(d3.curveMonotoneX)
        .x((d) => x(d.date))
        .y((d) => y(d.investment))
    );

  // Projected Net Worth line (solid)
  g.append("path")
    .datum(points)
    .style("stroke", COLORS.primary)
    .style("stroke-width", "2")
    .style("fill", "none")
    .attr(
      "d",
      d3
        .line<ProjectionPoint>()
        .curve(d3.curveMonotoneX)
        .x((d) => x(d.date))
        .y((d) => y(d.networth))
    );

  // Hover
  const hoverCircle = g.append("circle").attr("r", "3").attr("fill", "none");
  const t = tippy(hoverCircle.node(), { theme: "light", delay: 0, allowHTML: true });

  const voronoiNW: [number, number][] = points.map((d) => [x(d.date), y(d.networth)]);
  const voronoiInv: [number, number][] = points.map((d) => [x(d.date), y(d.investment)]);
  const voronoi = d3.Delaunay.from(voronoiNW.concat(voronoiInv)).voronoi([
    0,
    0,
    width,
    height
  ]);

  const labelFmt = d3.timeFormat("%b %Y");

  g.append("g")
    .selectAll("path")
    .data(
      (
        points.map((p) => ["networth", p] as ["networth" | "investment", ProjectionPoint])
      ).concat(
        points.map((p) => ["investment", p] as ["networth" | "investment", ProjectionPoint])
      )
    )
    .enter()
    .append("path")
    .style("pointer-events", "all")
    .style("fill", "none")
    .attr("d", (_, i) => voronoi.renderCell(i))
    .on("mouseover", (_, [type, d]) => {
      const cy = type === "networth" ? y(d.networth) : y(d.investment);
      const color = type === "networth" ? COLORS.primary : COLORS.secondary;
      hoverCircle.attr("cx", x(d.date)).attr("cy", cy).attr("fill", color);
      t.setProps({
        placement: type === "networth" ? "top" : "bottom",
        content: tooltip([
          ["Month", labelFmt(d.date)],
          [
            "Projected Net Worth",
            [formatCurrency(d.networth), "has-text-weight-bold has-text-right"]
          ],
          [
            "Projected Net Investment",
            [formatCurrency(d.investment), "has-text-weight-bold has-text-right"]
          ]
        ])
      });
      t.show();
    })
    .on("mouseout", () => {
      t.hide();
      hoverCircle.attr("fill", "none");
    });

  const legends: Legend[] = [
    { label: "Projected Net Worth", color: COLORS.primary, shape: "line" },
    { label: "Projected Net Investment", color: COLORS.secondary, shape: "line" },
    { label: "Market Gain", color: COLORS.gain, shape: "square" }
  ];

  return { destroy: () => t.destroy(), legends };
}
