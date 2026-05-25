import Swal from 'sweetalert2';

export const sideAlert = Swal.mixin({
  theme: 'auto',
  toast: true,
  position: 'bottom-end',
  showConfirmButton: false,
  timer: 3000,
  timerProgressBar: true,
  didOpen: (alert: HTMLElement) => {
    alert.addEventListener('mouseenter', Swal.stopTimer);
    alert.addEventListener('mouseleave', Swal.resumeTimer);
  },
});

export const centerAlert = Swal.mixin({
  theme: 'auto',
});
