import {
    AppBar,
    Toolbar,
    Typography,
    Box,
    ToggleButton,
    ToggleButtonGroup,
    Button
} from "@mui/material";
import { Link } from "react-router-dom";
import { useFlow } from "../context/FlowContext";
import { useAuth } from "../context/AuthContext";

export default function NavBar() {
    const { flow, setFlow } = useFlow();
    const { customerId, logout } = useAuth();

    return (
        <AppBar position="static" color="primary" elevation={1}>
            <Toolbar
                sx={{
                    gap: 3,
                    "& a": { textDecoration: "none", color: "inherit" }
                }}
            >
                <Typography variant="h6" component={Link} to="/">
                    Saga Demo
                </Typography>

                <Box sx={{ flexGrow: 1, display: "flex", gap: 2 }}>
                    <Button component={Link} to="/catalog" color="inherit">
                        Catalog
                    </Button>
                    <Button component={Link} to="/create" color="inherit">
                        New Order
                    </Button>
                    <Button component={Link} to="/status" color="inherit">
                        Order Status
                    </Button>
                </Box>

                <ToggleButtonGroup
                    size="small"
                    value={flow}
                    exclusive
                    onChange={(_, v) => v && setFlow(v)}
                    sx={{ "& .MuiToggleButton-root": { textTransform: "none", px: 1.5 } }}
                >
                    <ToggleButton value="choreographed">Choreography</ToggleButton>
                    <ToggleButton value="orchestrated">Orchestration</ToggleButton>
                </ToggleButtonGroup>

                <Box sx={{ ml: 3 }}>
                    {customerId ? (
                        <Button color="inherit" onClick={logout}>
                            Logout
                        </Button>
                    ) : (
                        <Button component={Link} to="/login" color="inherit">
                            Login / Register
                        </Button>
                    )}
                </Box>
            </Toolbar>
        </AppBar>
    );
}