"use strict";

/**
 * Pre-requirements: sweetAlert2
 * Don't forget to import the lib in the template,
 * i.e., <script src="/static/sweetalert2.all.min.js?version={{.Version}}"></script>
 * @reference https://sweetalert2.github.io
 * @icons [success, error, warning, info, question]
 * @example centerAlert.fire( { icon: "warning", title: "Warning", text: "err msg" } );
 * @example sideAlert.fire({ icon: "error", title: "Error", text: "Invalid birthday format" });
 * @author cwc1222
 */

/**@type {import("sweetalert2").default} */
const swal = Swal; // @ts-ignore

const sideAlert = swal.mixin({
    theme: 'auto',
    toast: true,
    position: 'bottom-end',
    showConfirmButton: false,
    timer: 3000,
    timerProgressBar: true,
    didOpen: (alert) => {
        alert.addEventListener('mouseenter', swal.stopTimer);
        alert.addEventListener('mouseleave', swal.resumeTimer);
    }
});
const centerAlert = swal.mixin({
    theme: 'auto',
});
