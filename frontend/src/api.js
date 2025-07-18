import axios from "axios";
import { useFlow } from "./context/FlowContext";

const api = axios.create({
    baseURL: process.env.REACT_APP_API_BASE_URL,
    timeout: 15000
});

/* ---------- header cliente ---------- */
export const setCustomerIdHeader = (id) => {
    if (id) api.defaults.headers.common["X-Customer-ID"] = id;
    else delete api.defaults.headers.common["X-Customer-ID"];
};

/* ---------- hook che astrae tutte le rotte dipendenti dal flusso ---------- */
export const useOrderApi = () => {
    const { flow } = useFlow();
    const flowParam = { params: { flow } };

    return {
        flow,

        /* catalogo */
        fetchCatalog: () => api.get("/catalog", flowParam),

        /* ordini */
        createOrder: (body) =>
            api.post(
                flow === "choreographed"
                    ? "/choreographed_order"
                    : "/orchestrated_order",
                body
            ),
        fetchAllOrders: (customerId) =>
            api.get("/orders", {
                params: { customer_id: customerId, flow }
            }),
        fetchOrder: (orderId) => api.get(`/orders/${orderId}`, flowParam)
    };
};

/* ---------- auth ---------- */
export const register = (username, password, name, email) =>
    api.post("/register", { username, password, name, email });

export const login = (username, password) =>
    api.post("/login", { username, password });
