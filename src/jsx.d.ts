declare module 'preact/jsx-runtime' {
  namespace JSX {
    interface IntrinsicElements {
      'iconify-icon': {
        icon?: string
        width?: number | string
        height?: number | string
      }
    }
  }
}

export {}
