// Card tilt effect
document.addEventListener('DOMContentLoaded', function() {
    // Apply tilt effect to cards
    const cards = document.querySelectorAll('.border-2.border-black.shadow-\\[5px_5px_0px_\\#000\\]');
    
    cards.forEach(card => {
        card.addEventListener('mousemove', function(e) {
            const rect = this.getBoundingClientRect();
            const x = e.clientX - rect.left;
            const y = e.clientY - rect.top;
            
            const centerX = rect.width / 2;
            const centerY = rect.height / 2;
            
            const rotateX = (y - centerY) / 20;
            const rotateY = (centerX - x) / 20;
            
            // Store the original transform
            const originalTransform = this.style.transform || '';
            
            // Apply the new transform, preserving original transform properties if any
            this.style.transform = `perspective(1000px) rotateX(${rotateX}deg) rotateY(${rotateY}deg)`;
        });
        
        card.addEventListener('mouseleave', function() {
            // Reset to original rotation but keep other transition effects
            this.style.transform = this.classList.contains('rotate-1') ? 'rotate(1deg)' : 
                                   this.classList.contains('-rotate-1') ? 'rotate(-1deg)' : '';
        });
    });
});