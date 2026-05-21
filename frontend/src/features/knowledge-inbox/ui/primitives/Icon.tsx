import type { ReactNode } from "react";
import type { IconName } from "../../presentation/viewModel";

const paths: Record<IconName, ReactNode> = {
  key: (
    <>
      <circle cx="7.5" cy="12.5" r="3.5" />
      <path d="M11 12.5h8m-3 0v3m-3-3v2" />
    </>
  ),
  x: (
    <>
      <path d="M5 5l14 14M19 5L5 19" />
    </>
  ),
  youtube: (
    <>
      <rect x="3" y="6.5" width="18" height="11" rx="3" />
      <path d="M10.5 9.5l5 2.5-5 2.5z" />
    </>
  ),
  check: (
    <>
      <path d="M20 6L9 17l-5-5" />
    </>
  ),
  run: (
    <>
      <path d="M4 12a8 8 0 0 1 13.6-5.7" />
      <path d="M18 3v5h-5" />
      <path d="M20 12a8 8 0 0 1-13.6 5.7" />
      <path d="M6 21v-5h5" />
    </>
  ),
  link: (
    <>
      <path d="M10 13a5 5 0 0 0 7.1 0l2-2a5 5 0 0 0-7.1-7.1l-1.1 1.1" />
      <path d="M14 11a5 5 0 0 0-7.1 0l-2 2A5 5 0 0 0 12 20.1l1.1-1.1" />
    </>
  ),
  alert: (
    <>
      <path d="M12 4l9 16H3z" />
      <path d="M12 9v5" />
      <path d="M12 17h.01" />
    </>
  ),
  spark: (
    <>
      <path d="M12 3l1.8 5.2L19 10l-5.2 1.8L12 17l-1.8-5.2L5 10l5.2-1.8z" />
      <path d="M19 15l.8 2.2L22 18l-2.2.8L19 21l-.8-2.2L16 18l2.2-.8z" />
    </>
  )
};

export function Icon({ name }: { name: IconName }) {
  return (
    <svg aria-hidden="true" className="icon-svg" viewBox="0 0 24 24">
      {paths[name]}
    </svg>
  );
}
