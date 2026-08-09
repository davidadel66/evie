// The design's inline SVGs, collected. Every one is 24x24 stroke-only so size
// and colour come from props — `currentColor` by default so a parent's text
// colour carries through.

type Props = {
  size?: number;
  stroke?: string;
  width?: number;
  className?: string;
};

// The factory trips oxlint's fast-refresh rule (a component built by a
// function, not declared). Accepted: icons are static, so losing HMR on this
// one file costs nothing, and a file per icon would be noise.
function svg(path: React.ReactNode, defaultWidth = 2) {
  return function Icon({ size = 13, stroke, width, className }: Props) {
    return (
      <svg
        width={size}
        height={size}
        viewBox="0 0 24 24"
        fill="none"
        stroke={stroke ?? "currentColor"}
        strokeWidth={width ?? defaultWidth}
        className={className}
        style={{ display: "block", flex: "none" }}
      >
        {path}
      </svg>
    );
  };
}

export const Wrench = svg(
  <path d="M14.7 6.3a4.5 4.5 0 1 0-6.4 6.4L3 18v3h3l5.3-5.3a4.5 4.5 0 0 0 6.4-6.4l-3 3-2-2 3-3z" />,
  1.8,
);

export const ChevronDown = svg(<path d="M6 9l6 6 6-6" />);
export const ChevronRight = svg(<path d="M9 18l6-6-6-6" />);
export const ChevronLeft = svg(<path d="M15 18l-6-6 6-6" />);
export const Check = svg(<path d="M20 6L9 17l-5-5" />, 2.2);
export const Cross = svg(<path d="M18 6L6 18M6 6l12 12" />, 2.2);

export const Alert = svg(
  <>
    <circle cx="12" cy="12" r="9" />
    <path d="M12 8v4M12 16h.01" />
  </>,
);

export const FileIcon = svg(
  <>
    <path d="M13 3H6a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V10z" />
    <path d="M13 3v7h7" />
  </>,
  1.8,
);

export const ArrowUp = svg(<path d="M12 19V5M5 12l7-7 7 7" />, 2.2);
