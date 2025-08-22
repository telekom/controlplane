// Add platform elements to slides
import { defineSetupVue3 } from '@slidev/types'

export default defineSetupVue3(({ app, router }) => {
  // Wait for the DOM to be fully loaded
  if (typeof document !== 'undefined') {
    const createPlatformElements = () => {
      const layouts = document.querySelectorAll('.slidev-layout')
      
      layouts.forEach(layout => {
        // Check if element already exists to avoid duplicates
        if (!layout.querySelector('.platform-element-top')) {
          const platformElementTop = document.createElement('div')
          platformElementTop.className = 'platform-element-top'
          layout.appendChild(platformElementTop)
        }
      })
    }

    // Initial creation
    router.afterEach(() => {
      setTimeout(createPlatformElements, 100)
    })
    
    // Add on slide change
    window.addEventListener('slidev:nav-changed', () => {
      setTimeout(createPlatformElements, 100)
    })
  }
})