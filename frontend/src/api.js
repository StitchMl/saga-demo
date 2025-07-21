import axios from "axios";
import { useFlow } from "./context/FlowContext";
import { useCallback } from "react";

const api = axios.create({
    baseURL: process.env.REACT_APP_API_BASE_URL,
    timeout: 15000
});

// Sets the header at the start of the app if the user is already logged in.
const initialCustomerId = sessionStorage.getItem("customerId");
if (initialCustomerId) {
    api.defaults.headers.common["X-Customer-ID"] = initialCustomerId;
}

/* ---------- customer header ---------- */
export const setCustomerIdHeader = (id) => {
    if (id) api.defaults.headers.common["X-Customer-ID"] = id;
    else delete api.defaults.headers.common["X-Customer-ID"];
};

/* ---------- hook that abstracts all flow-dependent routes ---------- */
export const useOrderApi = () => {
    const { flow } = useFlow();

    const fetchCatalog = useCallback(() => {
        return api.get("/catalog", { params: { flow } });
    }, [flow]);

    const createOrder = useCallback(
        (body) => {
            return api.post("/orders", body, { params: { flow } });
        },
        [flow]
    );

    const fetchAllOrders = useCallback(
        (customerId) => {
            return api.get("/orders", {
                params: { customer_id: customerId, flow }
            });
        },
        [flow]
    );

    const fetchOrder = useCallback(
        (orderId) => {
            return api.get(`/orders/${orderId}`, { params: { flow } });
        },
        [flow]
    );

    return {
        flow,
        fetchCatalog,
        createOrder,
        fetchAllOrders,
        fetchOrder
    };
};

/* ---------- auth ---------- */
export const register = (username, password, name, email, flow) =>
    api.post("/register", { username, password, name, email }, { params: { flow } });

export const login = (username, password, flow) =>
    api.post("/login", { username, password }, { params: { flow } });

export const validateCustomer = (customerId, flow) =>
    api.post("/validate", { customer_id: customerId }, { params: { flow } });
