// The icons the web app draws, copied from Phosphor 2.1.1 (MIT licence,
// notice in LICENSES/phosphor-MIT.txt), from the @phosphor-icons/core
// package. Interface icons are its Regular weight; a project's icon is its
// Duotone weight. Each takes the colour of the text around it, and the
// size its class gives it. They are drawn for the eye only: a button that
// holds one alone is named by its own label.
import type { ReactNode } from "react"

function icon(paths: ReactNode) {
  return function Icon({ className }: { className?: string }) {
    return (
      <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 256 256" fill="currentColor" aria-hidden="true" {...(className !== undefined && { className })}>
        {paths}
      </svg>
    )
  }
}

export const Plus = icon(<path d="M224,128a8,8,0,0,1-8,8H136v80a8,8,0,0,1-16,0V136H40a8,8,0,0,1,0-16h80V40a8,8,0,0,1,16,0v80h80A8,8,0,0,1,224,128Z" />)

export const CaretDown = icon(<path d="M213.66,101.66l-80,80a8,8,0,0,1-11.32,0l-80-80A8,8,0,0,1,53.66,90.34L128,164.69l74.34-74.35a8,8,0,0,1,11.32,11.32Z" />)

export const CaretRight = icon(<path d="M181.66,133.66l-80,80a8,8,0,0,1-11.32-11.32L164.69,128,90.34,53.66a8,8,0,0,1,11.32-11.32l80,80A8,8,0,0,1,181.66,133.66Z" />)

export const DotsThree = icon(
  <path d="M140,128a12,12,0,1,1-12-12A12,12,0,0,1,140,128Zm56-12a12,12,0,1,0,12,12A12,12,0,0,0,196,116ZM60,116a12,12,0,1,0,12,12A12,12,0,0,0,60,116Z" />,
)

export const MagnifyingGlass = icon(
  <path d="M229.66,218.34l-50.07-50.06a88.11,88.11,0,1,0-11.31,11.31l50.06,50.07a8,8,0,0,0,11.32-11.32ZM40,112a72,72,0,1,1,72,72A72.08,72.08,0,0,1,40,112Z" />,
)

export const FolderDuotone = icon(
  <>
    <path d="M128,80H32V56a8,8,0,0,1,8-8H92.69a8,8,0,0,1,5.65,2.34Z" opacity="0.2" />
    <path d="M216,72H131.31L104,44.69A15.86,15.86,0,0,0,92.69,40H40A16,16,0,0,0,24,56V200.62A15.4,15.4,0,0,0,39.38,216H216.89A15.13,15.13,0,0,0,232,200.89V88A16,16,0,0,0,216,72ZM92.69,56l16,16H40V56ZM216,200H40V88H216Z" />
  </>,
)
