import React, { useState } from 'react';
import { Send, CheckCircle, XCircle, Loader, Plus, Minus } from 'lucide-react'; // Importa le icone

function App() {
    // Stato per l'input dell'autenticazione
    const [customerId, setCustomerId] = useState('valid-customer'); // Valore predefinito per comodità

    // Stato per i dettagli dell'ordine (supporta più articoli)
    const [orderItems, setOrderItems] = useState([{ productId: 'product-A', quantity: 2 }]);

    // Stato per la selezione del tipo di SAGA
    const [sagaType, setSagaType] = useState('choreographed'); // 'choreographed' o 'orchestrated'

    // Stato per il caricamento e la risposta dell'API
    const [isLoading, setIsLoading] = useState(false);
    const [response, setResponse] = useState(null);
    const [error, setError] = useState(null);

    // Gestisce l'aggiunta di un nuovo articolo all'ordine
    const handleAddItem = () => {
        setOrderItems([...orderItems, { productId: '', quantity: 1 }]);
    };

    // Gestisce la rimozione di un articolo dall'ordine
    const handleRemoveItem = (index) => {
        const newItems = orderItems.filter((_, i) => i !== index);
        setOrderItems(newItems);
    };

    // Gestisce il cambiamento dei valori degli articoli
    const handleItemChange = (index, field, value) => {
        const newItems = orderItems.map((item, i) =>
            i === index ? { ...item, [field]: value } : item
        );
        setOrderItems(newItems);
    };

    // Gestisce l'invio del form
    const handleSubmit = async (e) => {
        e.preventDefault(); // Previene il ricaricamento della pagina

        setIsLoading(true);
        setResponse(null);
        setError(null);

        // Costruisci l'URL dell'API in base al tipo di SAGA selezionato
        const apiUrl = sagaType === 'choreographed'
            ? 'http://localhost:8000/choreographed_order'
            : 'http://localhost:8000/orchestrated_order';

        // Prepara il payload della richiesta
        const payload = {
            customer_id: customerId, // Il gateway lo sovrascriverà con quello autenticato dall'header
            items: orderItems.map(item => ({
                product_id: item.productId,
                quantity: parseInt(item.quantity, 10), // Assicurati che la quantità sia un numero intero
            })),
        };

        try {
            const res = await fetch(apiUrl, {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json',
                    'X-Customer-ID': customerId, // Header per l'autenticazione
                },
                body: JSON.stringify(payload),
            });

            const data = await res.json();

            if (!res.ok) {
                // Se la risposta non è OK (es. 400, 401, 500)
                setError(data.message || `Errore HTTP: ${res.status}`);
            } else {
                setResponse(data);
            }
        } catch (err) {
            setError(`Errore di connessione: ${err.message}`);
        } finally {
            setIsLoading(false);
        }
    };

    return (
        <div className="min-h-screen bg-gradient-to-br from-blue-50 to-indigo-100 flex items-center justify-center p-4">
            <div className="bg-white p-8 rounded-xl shadow-2xl w-full max-w-2xl border border-gray-200">
                <h1 className="text-4xl font-bold text-center text-indigo-800 mb-8 flex items-center justify-center">
                    <Send className="mr-3 text-indigo-600" size={36} />
                    SAGA Demo Frontend
                </h1>

                <form onSubmit={handleSubmit} className="space-y-6">
                    {/* Sezione Autenticazione */}
                    <div className="bg-indigo-50 p-4 rounded-lg border border-indigo-200 shadow-inner">
                        <label htmlFor="customerId" className="block text-sm font-medium text-indigo-700 mb-2">
                            ID Cliente (per Autenticazione X-Customer-ID):
                        </label>
                        <input
                            type="text"
                            id="customerId"
                            className="w-full p-3 border border-indigo-300 rounded-lg focus:ring-indigo-500 focus:border-indigo-500 text-gray-800"
                            value={customerId}
                            onChange={(e) => setCustomerId(e.target.value)}
                            placeholder="es. valid-customer o unauthorized-user"
                            required
                        />
                    </div>

                    {/* Sezione Articoli dell'Ordine */}
                    <div className="bg-blue-50 p-4 rounded-lg border border-blue-200 shadow-inner">
                        <h2 className="text-lg font-semibold text-blue-800 mb-4">Articoli dell'Ordine:</h2>
                        {orderItems.map((item, index) => (
                            <div key={index} className="flex items-center space-x-3 mb-4 last:mb-0">
                                <input
                                    type="text"
                                    className="flex-1 p-3 border border-blue-300 rounded-lg focus:ring-blue-500 focus:border-blue-500 text-gray-800"
                                    placeholder="ID Prodotto (es. product-A)"
                                    value={item.productId}
                                    onChange={(e) => handleItemChange(index, 'productId', e.target.value)}
                                    required
                                />
                                <input
                                    type="number"
                                    className="w-24 p-3 border border-blue-300 rounded-lg focus:ring-blue-500 focus:border-blue-500 text-gray-800"
                                    placeholder="Quantità"
                                    value={item.quantity}
                                    onChange={(e) => handleItemChange(index, 'quantity', e.target.value)}
                                    min="1"
                                    required
                                />
                                <button
                                    type="button"
                                    onClick={() => handleRemoveItem(index)}
                                    className="p-2 bg-red-500 text-white rounded-full hover:bg-red-600 transition duration-200 shadow-md"
                                    title="Rimuovi articolo"
                                >
                                    <Minus size={20} />
                                </button>
                            </div>
                        ))}
                        <button
                            type="button"
                            onClick={handleAddItem}
                            className="w-full flex items-center justify-center p-3 mt-4 bg-blue-600 text-white rounded-lg hover:bg-blue-700 transition duration-200 shadow-md"
                        >
                            <Plus size={20} className="mr-2" /> Aggiungi Articolo
                        </button>
                    </div>

                    {/* Sezione Tipo di SAGA */}
                    <div className="bg-green-50 p-4 rounded-lg border border-green-200 shadow-inner">
                        <h2 className="text-lg font-semibold text-green-800 mb-4">Seleziona Tipo di SAGA:</h2>
                        <div className="flex space-x-6">
                            <label className="inline-flex items-center">
                                <input
                                    type="radio"
                                    className="form-radio text-green-600 h-5 w-5"
                                    name="sagaType"
                                    value="choreographed"
                                    checked={sagaType === 'choreographed'}
                                    onChange={(e) => setSagaType(e.target.value)}
                                />
                                <span className="ml-2 text-gray-700 font-medium">Coreografica</span>
                            </label>
                            <label className="inline-flex items-center">
                                <input
                                    type="radio"
                                    className="form-radio text-green-600 h-5 w-5"
                                    name="sagaType"
                                    value="orchestrated"
                                    checked={sagaType === 'orchestrated'}
                                    onChange={(e) => setSagaType(e.target.value)}
                                />
                                <span className="ml-2 text-gray-700 font-medium">Orchestrata</span>
                            </label>
                        </div>
                    </div>

                    {/* Pulsante Invia */}
                    <button
                        type="submit"
                        className="w-full flex items-center justify-center p-4 bg-indigo-600 text-white font-semibold rounded-lg shadow-lg hover:bg-indigo-700 transition duration-300 disabled:opacity-50 disabled:cursor-not-allowed"
                        disabled={isLoading}
                    >
                        {isLoading ? (
                            <>
                                <Loader className="animate-spin mr-3" size={24} /> Invio in corso...
                            </>
                        ) : (
                            <>
                                <Send className="mr-3" size={24} /> Invia Ordine
                            </>
                        )}
                    </button>
                </form>

                {/* Sezione Risposta */}
                {isLoading && (
                    <div className="mt-8 text-center text-indigo-600 flex items-center justify-center">
                        <Loader className="animate-spin mr-3" size={24} /> Elaborazione...
                    </div>
                )}

                {error && (
                    <div className="mt-8 p-4 bg-red-100 border border-red-400 text-red-700 rounded-lg flex items-center shadow-md">
                        <XCircle className="mr-3" size={24} />
                        <div>
                            <h3 className="font-semibold text-lg mb-1">Errore:</h3>
                            <p>{error}</p>
                        </div>
                    </div>
                )}

                {response && !error && (
                    <div className="mt-8 p-4 bg-green-100 border border-green-400 text-green-700 rounded-lg flex items-center shadow-md">
                        <CheckCircle className="mr-3" size={24} />
                        <div>
                            <h3 className="font-semibold text-lg mb-1">Risposta API:</h3>
                            <pre className="whitespace-pre-wrap text-sm">{JSON.stringify(response, null, 2)}</pre>
                        </div>
                    </div>
                )}
            </div>
        </div>
    );
}

export default App;