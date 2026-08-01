import { render } from 'preact'
import { App } from './app'
import '@fontsource/inria-sans/400.css'
import '@fontsource/inria-sans/700.css'
import '@fontsource/inria-sans/400-italic.css'
import '@fontsource/jetbrains-mono/400.css'
import '@fontsource/jetbrains-mono/600.css'

render(<App />, document.getElementById('app')!)
