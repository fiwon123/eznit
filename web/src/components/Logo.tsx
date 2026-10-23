interface LogoProps {
  className?: string;
}

export default function Logo({ className }: LogoProps) {
  return (
    <svg
      className={className}
      xmlns="http://www.w3.org/2000/svg"
      width="128"
      height="32"
      viewBox="0 0 128 32"
      role="img"
      aria-label="eznit"
    >
      <rect x="2" y="2" width="28" height="28" rx="8" fill="#6ea8fe" />
      <g
        fill="none"
        stroke="#ffffff"
        strokeWidth="2"
        strokeLinecap="round"
        strokeLinejoin="round"
      >
        <path d="M10 12h12v10H10z" />
        <path d="M12 10h12v10" />
        <polyline points="16 16 19 19 16 22" />
      </g>
      <text
        x="38"
        y="22"
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
