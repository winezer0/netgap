import {createApp} from 'vue'
import App from './App.vue'
import { backend } from './services/backend'
import './style.css';

createApp(App).mount('#app')
backend.frontendReady()
