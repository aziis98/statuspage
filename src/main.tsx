import { render } from 'preact'
import { App } from './app'
import '@fontsource/inria-sans/400.css'
import '@fontsource/inria-sans/700.css'
import '@fontsource/inria-sans/400-italic.css'
import '@fontsource/iosevka-etoile/400.css'
import '@fontsource/iosevka-etoile/600.css'
import './styles.css'

render(<App />, document.getElementById('app')!)
