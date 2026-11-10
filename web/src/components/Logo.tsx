interface LogoProps {
  className?: string;
}

// The mark is decorative: it carries no accessible name of its own, so the
// element that is actually interactive has to name itself (the brand link in
// App.tsx). An inline <svg> cannot be replaced by an <img> here — see the note
// in App.tsx — and a role="img" would only duplicate the link's name.
//
// A starting dot and two forward chevrons: "start, quickly" (eznit = easy
// init). The geometry is the 512 px mark in assets/logo.svg scaled to a 28 px
// tile; keep the two files in step.
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
      <path
        d="M 17.37 9.98 L 25.57 16 L 17.37 22.02 L 17.37 17.97 L 21.25 16 L 17.37 14.03 Z"
        fill="#93C5FD"
      />
      <path
        d="M 11.02 9.98 L 19.23 16 L 11.02 22.02 L 11.02 17.97 L 14.91 16 L 11.02 14.03 Z"
        fill="#ffffff"
      />
      <circle cx="8.07" cy="16" r="1.64" fill="#ffffff" />
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
