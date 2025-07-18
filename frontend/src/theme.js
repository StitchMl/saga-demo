import { createTheme } from "@mui/material/styles";

const theme = createTheme({
    palette: {
        primary: { main: "#1976d2" },
        secondary: { main: "#d32f2f" }
    },
    shape: { borderRadius: 12 },
    components: {
        MuiButton: { styleOverrides: { root: { textTransform: "none" } } }
    }
});

export default theme;