interface LogoProps {
  className?: string;
}

// The mark is decorative: it carries no accessible name of its own, so the
// element that is actually interactive has to name itself (the brand link in
// App.tsx). An inline <svg> cannot be replaced by an <img> here — see the note
// in App.tsx — and a role="img" would only duplicate the link's name.
//
// The E is four rounded bars: a monogram that also reads as stacked files. The
// geometry is the 512 px mark in assets/logo.svg scaled to a 28 px tile; keep
// the two files in step.
export default function Logo({ className }: LogoProps) {
  return (
    <svg
      className={className}
      xmlns="http://www.w3.org/2000/svg"
      width="128"
      height="32"
      viewBox="0 0 128 32"
      aria-hidden="true"
    >
      <rect x="2" y="2" width="28" height="28" rx="6.13" fill="#2563EB" />
      <g fill="#ffffff">
        <rect x="9.11" y="9.66" width="3.28" height="12.69" rx="1.64" />
        <rect x="9.11" y="9.66" width="13.78" height="3.28" rx="1.64" />
        <rect x="9.11" y="14.36" width="11.16" height="3.28" rx="1.64" />
        <rect x="9.11" y="19.06" width="13.78" height="3.28" rx="1.64" />
      </g>
      <text
        x="38"
        y="22.5"
        fontFamily="Inter, system-ui, -apple-system, sans-serif"
        fontSize="18"
        fontWeight="600"
        fill="currentColor"
      >
        eznit
      </text>
    </svg>
  );
}
